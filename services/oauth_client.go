package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"antigravity-bridge/config"
)

const (
	GoogleOAuthRedirectURI = "http://127.0.0.1:4040/oauth2callback"
	GoogleOAuthScope       = "https://www.googleapis.com/auth/cloud-platform https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/userinfo.profile openid"
)

func GetGoogleOAuthRedirectURI() string {
	if custom := os.Getenv("GOOGLE_OAUTH_REDIRECT_URI"); custom != "" {
		return custom
	}
	port := config.AppConfig.Port
	if port == "" {
		port = "4040"
	}
	return fmt.Sprintf("http://127.0.0.1:%s/oauth2callback", port)
}

func getGoogleOAuthClientID() string {
	if config.AppConfig.GoogleClientID != "" {
		return config.AppConfig.GoogleClientID
	}
	return os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
}

func getGoogleOAuthClientSecret() string {
	if config.AppConfig.GoogleClientSecret != "" {
		return config.AppConfig.GoogleClientSecret
	}
	return os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
}

// GetGoogleAuthURL generates the direct Google OAuth URL for browser authentication.
func GetGoogleAuthURL() string {
	clientID := getGoogleOAuthClientID()
	if clientID == "" {
		return ""
	}
	redirectURI := GetGoogleOAuthRedirectURI()
	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", GoogleOAuthScope)
	params.Set("access_type", "offline")
	params.Set("prompt", "consent")
	query := strings.ReplaceAll(params.Encode(), "+", "%20")
	return "https://accounts.google.com/o/oauth2/v2/auth?" + query
}

// ExchangeGoogleAuthCode exchanges the authorization code with Google for tokens and registers the profile.
func ExchangeGoogleAuthCode(code string) (*AccountProfile, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, fmt.Errorf("código de autorização não fornecido")
	}

	clientID := getGoogleOAuthClientID()
	clientSecret := getGoogleOAuthClientSecret()
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("credenciais Google OAuth não configuradas no servidor (defina GOOGLE_OAUTH_CLIENT_ID e GOOGLE_OAUTH_CLIENT_SECRET no .env)")
	}

	redirectURI := GetGoogleOAuthRedirectURI()
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar com o Google OAuth: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler resposta do Google: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		var gErr struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &gErr)
		if gErr.ErrorDescription != "" {
			return nil, fmt.Errorf("erro do Google: %s (%s)", gErr.ErrorDescription, gErr.Error)
		}
		return nil, fmt.Errorf("erro do Google (%d): %s", resp.StatusCode, string(body))
	}

	// Register profile and switch to it
	return ImportAccount(body)
}
