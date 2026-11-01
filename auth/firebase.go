package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"antigravity-bridge/config"
	"github.com/golang-jwt/jwt/v5"
)

const googleCertURL = "https://www.googleapis.com/robot/v1/metadata/x509/securetoken@system.gserviceaccount.com"

type certCache struct {
	sync.RWMutex
	certs     map[string]*rsa.PublicKey
	expiresAt time.Time
}

var cache = certCache{
	certs: make(map[string]*rsa.PublicKey),
}

type FirebaseUser struct {
	UID   string `json:"uid"`
	Email string `json:"email"`
}

func fetchPublicKeys() (map[string]*rsa.PublicKey, time.Time, error) {
	resp, err := http.Get(googleCertURL)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("failed to fetch certs: status %d", resp.StatusCode)
	}

	var rawCerts map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&rawCerts); err != nil {
		return nil, time.Time{}, err
	}

	keys := make(map[string]*rsa.PublicKey)
	for kid, certPEM := range rawCerts {
		block, _ := pem.Decode([]byte(certPEM))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if pubKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			keys[kid] = pubKey
		}
	}

	// Cache for 1 hour by default
	return keys, time.Now().Add(1 * time.Hour), nil
}

func getPublicKey(kid string) (*rsa.PublicKey, error) {
	cache.RLock()
	if time.Now().Before(cache.expiresAt) {
		if key, exists := cache.certs[kid]; exists {
			cache.RUnlock()
			return key, nil
		}
	}
	cache.RUnlock()

	cache.Lock()
	defer cache.Unlock()

	// Double check
	if time.Now().Before(cache.expiresAt) {
		if key, exists := cache.certs[kid]; exists {
			return key, nil
		}
	}

	keys, expiry, err := fetchPublicKeys()
	if err != nil {
		return nil, err
	}
	cache.certs = keys
	cache.expiresAt = expiry

	key, ok := keys[kid]
	if !ok {
		return nil, errors.New("public key not found for kid")
	}
	return key, nil
}

func VerifyFirebaseToken(tokenString string) (*FirebaseUser, error) {
	if tokenString == "" {
		return nil, errors.New("token is empty")
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in token header")
		}
		return getPublicKey(kid)
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("cannot parse claims")
	}

	expectedIssuer := fmt.Sprintf("https://securetoken.google.com/%s", config.AppConfig.FirebaseProjectID)
	if iss, _ := claims["iss"].(string); iss != expectedIssuer {
		return nil, fmt.Errorf("invalid issuer: got %s, want %s", iss, expectedIssuer)
	}

	if aud, _ := claims["aud"].(string); aud != config.AppConfig.FirebaseProjectID {
		return nil, fmt.Errorf("invalid audience: got %s, want %s", aud, config.AppConfig.FirebaseProjectID)
	}

	uid, _ := claims["user_id"].(string)
	if uid == "" {
		uid, _ = claims["sub"].(string)
	}
	if uid == "" {
		return nil, errors.New("missing uid/sub in token")
	}

	email, _ := claims["email"].(string)
	emailVerified, _ := claims["email_verified"].(bool)
	if email != "" && !emailVerified {
		return nil, errors.New("e-mail não verificado pelo provedor de autenticação")
	}

	return &FirebaseUser{
		UID:   uid,
		Email: email,
	}, nil
}
