package gateway

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"antigravity-bridge/config"
	"antigravity-bridge/services"
)

func handleLandingPage(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Query().Get("download") == "1" || r.URL.Query().Get("action") == "apk" {
		handleAPKDownload(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	vInfo := services.LoadVersionData()
	ver := vInfo.Version
	if ver == "" {
		ver = "1.3.6"
	}
	build := vInfo.Build
	if build == 0 {
		build = 15
	}

	apkFileName := fmt.Sprintf("AntigravityMobile-v%s.apk", ver)
	html := strings.ReplaceAll(landingPageHTML, "AntigravityMobile.apk", apkFileName)
	html = strings.ReplaceAll(html, "{{VERSION}}", ver)
	html = strings.ReplaceAll(html, "{{BUILD}}", fmt.Sprintf("%d", build))
	_, _ = io.WriteString(w, html)
}

func handleAPKDownload(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var apkPath string

	vInfo := services.LoadVersionData()
	ver := vInfo.Version
	if ver == "" {
		ver = "1.3.6"
	}
	expectedVersioned := filepath.Join(filepath.Dir(config.AppConfig.VersionFile), fmt.Sprintf("AntigravityMobile-v%s.apk", ver))
	if fi, err := os.Stat(expectedVersioned); err == nil && fi.Size() > 5*1024*1024 {
		apkPath = expectedVersioned
	}

	if apkPath == "" {
		pattern := filepath.Join(filepath.Dir(config.AppConfig.VersionFile), "AntigravityMobile-v*.apk")
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			apkPath = matches[len(matches)-1]
		}
	}

	if apkPath == "" {
		for _, p := range config.AppConfig.ApkCandidates {
			fi, err := os.Stat(p)
			if err == nil && fi.Size() > 5*1024*1024 {
				apkPath = p
				break
			}
		}
	}

	if apkPath == "" {
		build := vInfo.Build
		if build == 0 {
			build = 24
		}
		releaseURL := fmt.Sprintf("https://github.com/WebuildSoft/antigravity-mobile-releases/releases/download/v%s/AntigravityMobile-v%s-build%d.apk", ver, ver, build)
		fmt.Printf("📦 [Download APK] Redirecionando para GitHub Release: %s\n", releaseURL)
		http.Redirect(w, r, releaseURL, http.StatusFound)
		return
	}

	filename := fmt.Sprintf("AntigravityMobile-v%s.apk", ver)
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.ServeFile(w, r, apkPath)
}

func handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_, _ = io.WriteString(w, `self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then(keys => Promise.all(keys.map(k => caches.delete(k))))
      .then(() => self.registration.unregister())
      .then(() => self.clients.matchAll({ type: 'window' }))
      .then(clients => clients.forEach(c => c.navigate(c.url)))
  );
});`)
}

func handleRootAndStatic(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	webDir := config.AppConfig.WebDir
	webDirAbs, err := filepath.Abs(webDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	p := filepath.Clean(r.URL.Path)
	if p == "/" || p == "." {
		p = "/index.html"
	}

	fullPath := filepath.Join(webDirAbs, p)
	if !strings.HasPrefix(fullPath, webDirAbs+string(filepath.Separator)) && fullPath != webDirAbs {
		http.NotFound(w, r)
		return
	}

	if strings.Contains(r.URL.RawPath, "..") || strings.Contains(r.RequestURI, "..") {
		http.NotFound(w, r)
		return
	}

	fi, err := os.Stat(fullPath)
	if err != nil || fi.IsDir() {
		indexFile := filepath.Join(webDirAbs, "index.html")
		if _, errIndex := os.Stat(indexFile); errIndex == nil {
			fullPath = indexFile
		} else {
			if p == "/index.html" || p == "/" {
				handleLandingPage(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	if ext == ".html" || ext == ".js" || ext == ".json" {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	}

	http.ServeFile(w, r, fullPath)
}

const landingPageHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Baixar Antigravity Mobile</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body {
      background: #0B0F19;
      color: #E2E8F0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
      min-height: 100vh;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 20px;
    }
    .card {
      background: #161F30;
      border: 1px solid #2E3A52;
      border-radius: 24px;
      padding: 32px 24px;
      max-width: 420px;
      width: 100%;
      text-align: center;
      box-shadow: 0 20px 40px rgba(0,0,0,0.5);
    }
    .icon {
      width: 90px;
      height: 90px;
      border-radius: 22px;
      margin: 0 auto 16px;
      display: block;
      box-shadow: 0 8px 24px rgba(99,102,241,0.4);
    }
    h1 { font-size: 22px; font-weight: 800; color: #FFFFFF; margin-bottom: 6px; }
    .badge {
      display: inline-block;
      background: rgba(99,102,241,0.2);
      color: #818CF8;
      border: 1px solid rgba(99,102,241,0.4);
      padding: 4px 12px;
      border-radius: 12px;
      font-size: 12px;
      font-weight: 600;
      margin-bottom: 20px;
    }
    .desc { font-size: 14px; color: #94A3B8; line-height: 1.5; margin-bottom: 28px; }
    .btn-download {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 10px;
      background: linear-gradient(135deg, #10B981, #059669);
      color: #FFFFFF;
      text-decoration: none;
      font-size: 16px;
      font-weight: 700;
      padding: 16px 20px;
      border-radius: 16px;
      box-shadow: 0 8px 20px rgba(16,185,129,0.35);
      margin-bottom: 12px;
    }
    .btn-wifi {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      background: #1E293B;
      color: #94A3B8;
      border: 1px solid #334155;
      text-decoration: none;
      font-size: 13px;
      font-weight: 600;
      padding: 12px 16px;
      border-radius: 12px;
      margin-bottom: 24px;
    }
    .instructions {
      background: #0F172A;
      border: 1px solid #1E293B;
      border-radius: 16px;
      padding: 16px;
      text-align: left;
      font-size: 12px;
      color: #94A3B8;
      line-height: 1.6;
    }
    .step { margin-bottom: 8px; display: flex; align-items: flex-start; gap: 8px; }
    .step:last-child { margin-bottom: 0; }
    .num {
      background: #334155;
      color: #FFF;
      font-weight: bold;
      border-radius: 50%;
      width: 18px;
      height: 18px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 10px;
      flex-shrink: 0;
      margin-top: 2px;
    }
  </style>
</head>
<body>
  <div class="card">
    <img src="/icons/Icon-192.png" alt="Antigravity Icon" class="icon">
    <h1>Antigravity Mobile</h1>
    <span class="badge">Versão v{{VERSION}} (Build #{{BUILD}}) • Servidor Go</span>
    <p class="desc">Nova versão com correção de comandos, sincronização instantânea e cotas em tempo real.</p>
    
    <a id="downloadBtn" href="/baixar?download=1" class="btn-download" download="AntigravityMobile-v{{VERSION}}.apk">
      <span>📲 BAIXAR APK v{{VERSION}}</span>
      <small style="opacity:0.85;font-size:12px;">(AntigravityMobile-v{{VERSION}}.apk)</small>
    </a>

    <a href="/app.apk" class="btn-wifi">
      <span>⚡ Download Direto Alternativo (.apk)</span>
    </a>

    <div class="instructions">
      <div class="step"><span class="num">1</span><span>Toque no botão verde acima para baixar o arquivo <strong>AntigravityMobile-v{{VERSION}}.apk</strong>.</span></div>
      <div class="step"><span class="num">2</span><span>Se o navegador avisar <em>"O arquivo pode ser nocivo"</em>, clique em <strong>Fazer o download mesmo assim</strong>.</span></div>
      <div class="step"><span class="num">3</span><span>Após baixar, abra o arquivo baixado e selecione <strong>Instalar</strong>.</span></div>
    </div>
  </div>
</body>
</html>`
