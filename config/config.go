package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	Port               string
	FirebaseProjectID  string
	AllowedEmails      []string
	AgyBin             string
	DefaultWorkspace   string
	WebDir             string
	BrainDir           string
	UploadsDir         string
	VersionFile        string
	ApkCandidates      []string
	PublicURL          string
	GoogleClientID     string
	GoogleClientSecret string
	ErrorWebhookURL    string
	TelemetryEnabled   bool
}

func loadEnvFile(envPath string) {
	data, err := os.ReadFile(envPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

var AppConfig Config

func findProjectRoot() string {
	if pr := os.Getenv("PROJECT_ROOT"); pr != "" {
		return pr
	}
	if cwd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "web")); err == nil {
			return cwd
		}
		if _, err := os.Stat(filepath.Join(cwd, "mobile/build/web")); err == nil {
			return cwd
		}
		if _, err := os.Stat(filepath.Join(cwd, "../web")); err == nil {
			abs, _ := filepath.Abs(filepath.Join(cwd, ".."))
			return abs
		}
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(exeDir, "web")); err == nil {
			return exeDir
		}
		if _, err := os.Stat(filepath.Join(exeDir, "../web")); err == nil {
			abs, _ := filepath.Abs(filepath.Join(exeDir, ".."))
			return abs
		}
	}
	cwd, _ := os.Getwd()
	return cwd
}

func detectAgyBinary(home string) string {
	if envAgy := os.Getenv("AGY_BIN"); envAgy != "" {
		return envAgy
	}
	if pathAgy, err := exec.LookPath("agy"); err == nil && pathAgy != "" {
		return pathAgy
	}
	if pathAgy, err := exec.LookPath("agy.exe"); err == nil && pathAgy != "" {
		return pathAgy
	}
	candidates := []string{
		filepath.Join(home, "AppData", "Local", "agy", "bin", "agy.exe"),
		filepath.Join(home, "AppData", "Local", "agy", "bin", "agy"),
		filepath.Join(home, ".local", "bin", "agy"),
		filepath.Join(home, ".local", "bin", "agy.exe"),
		filepath.Join(home, ".gemini", "antigravity-cli", "bin", "agy"),
		filepath.Join(home, ".gemini", "antigravity-cli", "bin", "agy.exe"),
		filepath.Join(home, "AppData", "Local", "Programs", "antigravity", "bin", "agy.exe"),
		filepath.Join(home, "AppData", "Local", "antigravity-cli", "bin", "agy.exe"),
		"/usr/local/bin/agy",
		"/usr/bin/agy",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return "agy"
}

func detectAllowedEmails() []string {
	rawEmails := os.Getenv("ALLOWED_EMAILS")
	if rawEmails != "" && rawEmails != "*" {
		parts := strings.Split(rawEmails, ",")
		var allowed []string
		for _, p := range parts {
			clean := strings.ToLower(strings.TrimSpace(p))
			if clean != "" {
				allowed = append(allowed, clean)
			}
		}
		if len(allowed) > 0 {
			return allowed
		}
	}

	// Por padrão, qualquer usuário que criar conta no app móvel e autenticar tem acesso liberado
	return []string{"*"}
}

func InitConfig() {
	rootDir := findProjectRoot()
	loadEnvFile(filepath.Join(rootDir, ".env"))

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
		if home == "" {
			home = "."
		}
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "4040"
	}

	fbProj := os.Getenv("FIREBASE_PROJECT_ID")
	if fbProj == "" {
		fbProj = "studio-5952025251-f9349"
	}

	allowed := detectAllowedEmails()
	agyBin := detectAgyBinary(home)

	workspace := os.Getenv("DEFAULT_WORKSPACE")
	if workspace == "" {
		workspace = home
	}

	webDir := os.Getenv("WEB_DIR")
	if webDir == "" {
		candidateWeb := filepath.Join(rootDir, "web")
		if _, err := os.Stat(candidateWeb); err == nil {
			webDir = candidateWeb
		} else {
			webDir = filepath.Join(rootDir, "mobile/build/web")
		}
	}

	publicURL := os.Getenv("PUBLIC_URL")
	googleClientID := os.Getenv("GOOGLE_OAUTH_CLIENT_ID")
	googleClientSecret := os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")
	errorWebhook := os.Getenv("ERROR_WEBHOOK_URL")
	if errorWebhook == "" {
		errorWebhook = os.Getenv("DISCORD_WEBHOOK_URL")
	}

	telemetryEnv := strings.ToLower(strings.TrimSpace(os.Getenv("TELEMETRY_ENABLED")))
	telemetryEnabled := telemetryEnv != "false" && telemetryEnv != "0" && telemetryEnv != "no"

	AppConfig = Config{
		Port:               port,
		FirebaseProjectID:  fbProj,
		AllowedEmails:      allowed,
		AgyBin:             agyBin,
		DefaultWorkspace:   workspace,
		WebDir:             webDir,
		BrainDir:           filepath.Join(home, ".gemini/antigravity-cli/brain"),
		UploadsDir:         filepath.Join(rootDir, "uploads"),
		VersionFile:        filepath.Join(rootDir, "version.json"),
		PublicURL:          publicURL,
		GoogleClientID:     googleClientID,
		GoogleClientSecret: googleClientSecret,
		ErrorWebhookURL:    errorWebhook,
		TelemetryEnabled:   telemetryEnabled,
		ApkCandidates: []string{
			filepath.Join(rootDir, "app-release.apk"),
			filepath.Join(rootDir, "uploads/app-release.apk"),
		},
	}

	_ = os.MkdirAll(AppConfig.UploadsDir, 0755)
	_ = os.MkdirAll(filepath.Join(rootDir, "data"), 0755)
}

func (c *Config) IsEmailAllowed(email string) bool {
	if email == "" {
		return false
	}
	clean := strings.ToLower(strings.TrimSpace(email))
	for _, a := range c.AllowedEmails {
		if a == "*" || a == clean {
			return true
		}
	}
	return false
}
