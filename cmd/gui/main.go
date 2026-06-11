package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	backendURL          = "http://localhost:58080"
	backendStartTimeout = 10 * time.Second
	ipcPort             = "127.0.0.1:58989" // single-instance IPC
	guiServePort        = "127.0.0.1:0"     // 0 = random available port
)

//go:embed frontend/dist
var frontendAssets embed.FS

func main() {
	// ── Single-instance check ──
	if conn, err := net.DialTimeout("tcp", ipcPort, 500*time.Millisecond); err == nil {
		conn.Write([]byte("show"))
		conn.Close()
		log.Println("another xbot-gui instance is running, sent 'show' and exiting")
		os.Exit(0)
	}

	listener, err := net.Listen("tcp", ipcPort)
	if err != nil {
		log.Fatalf("failed to bind IPC port %s: %v", ipcPort, err)
	}
	defer listener.Close()

	// ── Ensure xbot serve is running ──
	if !isServeRunning() {
		log.Println("xbot serve not running, starting...")
		startServe()
	}
	waitForServe()

	// ── Auto-login: get session token from backend API ──
	sessionToken := autoLogin()

	// ── Start local HTTP server for embedded frontend ──
	// Wails uses wails:// protocol which can't proxy API/WebSocket.
	// So we serve frontend via localhost HTTP + reverse proxy to backend.
	guiURL := startGUIServer(sessionToken)
	log.Println("GUI server listening on", guiURL)

	svc := &XbotService{}

	app := application.New(application.Options{
		Name:        "xbot",
		Description: "xbot - AI Assistant Desktop Client",
		Services: []application.Service{
			application.NewService(svc),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	svc.app = app

	// ── Window: load from local HTTP server ──
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "xbot",
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:             guiURL,
	})

	// ── System Tray ──
	tray := app.SystemTray.New()
	tray.SetIcon(generateTrayIcon())

	menu := app.NewMenu()
	showHide := menu.Add("显示/隐藏窗口")
	showHide.OnClick(func(ctx *application.Context) {
		if win.IsVisible() {
			win.Hide()
		} else {
			win.Show()
		}
	})
	menu.AddSeparator()
	settings := menu.Add("设置")
	settings.OnClick(func(ctx *application.Context) {
		win.Show()
		win.SetURL(guiURL + "/settings")
	})
	menu.AddSeparator()
	quit := menu.Add("退出")
	quit.OnClick(func(ctx *application.Context) {
		tray.Destroy()
		app.Quit()
	})
	tray.SetMenu(menu)
	tray.AttachWindow(win)

	// ── IPC: handle "show" from other instances ──
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 16)
			n, _ := conn.Read(buf)
			cmd := strings.TrimSpace(string(buf[:n]))
			switch cmd {
			case "show":
				win.Show()
			case "exit":
				tray.Destroy()
				app.Quit()
			}
			conn.Close()
		}
	}()

	tray.Show()
	log.Println("xbot-gui ready")

	if err := app.Run(); err != nil {
		tray.Destroy()
		log.Fatalf("application error: %v", err)
	}

	tray.Destroy()
}

// ── Local HTTP server ─────────────────

// startGUIServer starts a local HTTP server that:
// 1. Serves the embedded frontend files
// 2. Proxies /api/* and /ws to the xbot serve backend
// Returns the base URL (e.g. "http://127.0.0.1:51999") with optional #token.
func startGUIServer(sessionToken string) string {
	distFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	backendProxy := createBackendProxy()

	// Pre-read index.html bytes to avoid http.FileServer's redirect trap.
	// (FileServer redirects /index.html → / → our handler sets /index.html → loop)
	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		log.Fatalf("failed to read embedded index.html: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Proxy API and WebSocket to backend
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/ws") {
			backendProxy.ServeHTTP(w, r)
			return
		}

		// Serve index.html directly for SPA routes (/, /index.html, /settings, etc.)
		// IMPORTANT: must write bytes directly, NOT via http.FileServer,
		// because FileServer redirects /index.html → / causing an infinite loop.
		if path == "/" || path == "/index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexHTML)
			return
		}

		// For other paths, try static file first
		cleanPath := strings.TrimPrefix(path, "/")
		if f, err := distFS.Open(cleanPath); err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// SPA fallback: unknown routes → index.html
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	// Listen on random available port
	ln, err := net.Listen("tcp", guiServePort)
	if err != nil {
		log.Fatalf("failed to start GUI server: %v", err)
	}

	go http.Serve(ln, handler)

	base := "http://" + ln.Addr().String()
	if sessionToken != "" {
		return base + "/#token=" + sessionToken
	}
	return base + "/"
}

func createBackendProxy() http.Handler {
	target, _ := url.Parse(backendURL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	defaultDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		defaultDirector(req)
		req.Host = target.Host
	}
	return proxy
}

// ── Serve lifecycle ───────────────────

func isServeRunning() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(backendURL + "/api/history")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func startServe() {
	cmd := exec.Command("xbot-cli", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		log.Printf("failed to start xbot serve: %v", err)
	} else {
		log.Printf("xbot serve started (pid=%d)", cmd.Process.Pid)
	}
}

func waitForServe() {
	deadline := time.Now().Add(backendStartTimeout)
	for time.Now().Before(deadline) {
		if isServeRunning() {
			log.Println("xbot serve is ready")
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Println("timeout waiting for xbot serve, continuing anyway")
}

// ── Auto-login ────────────────────────

func autoLogin() string {
	user, pass := readCredentials()
	if user == "" || pass == "" {
		log.Println("auto-login: no credentials found")
		return ""
	}

	body, _ := json.Marshal(map[string]string{
		"username": user,
		"password": pass,
	})
	resp, err := http.Post(backendURL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("auto-login: request failed: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("auto-login: server returned %d", resp.StatusCode)
		return ""
	}

	for _, cookie := range resp.Cookies() {
		if cookie.Name == "xbot_session" {
			log.Println("auto-login: successful")
			return cookie.Value
		}
	}

	var result map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&result) == nil {
		if token, ok := result["token"].(string); ok {
			log.Println("auto-login: successful (from body)")
			return token
		}
	}

	log.Println("auto-login: no session token found in response")
	return ""
}

func readCredentials() (string, string) {
	if u := os.Getenv("XBOT_USER"); u != "" {
		return u, os.Getenv("XBOT_PASS")
	}
	data, err := os.ReadFile(os.ExpandEnv("$HOME/.xbot/gui_creds"))
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) >= 2 && lines[0] != "" && lines[1] != "" {
			return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
		}
	}
	return "", ""
}

// ── Tray Icon ─────────────────────────

func generateTrayIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	center, radius := 16, 14
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx := x - center
			dy := y - center
			d2 := dx*dx + dy*dy
			if d2 > radius*radius {
				continue
			}
			alpha := 255
			edge := (radius - 1) * (radius - 1)
			if d2 > edge {
				alpha = 255 - (d2-edge)*255/(2*radius)
				if alpha < 0 {
					alpha = 0
				}
			}
			img.Set(x, y, color.RGBA{R: 79, G: 195, B: 247, A: uint8(alpha)})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

// ── XbotService ───────────────────────

type XbotService struct {
	ctx context.Context
	app *application.App
}

func (s *XbotService) Startup(ctx context.Context) { s.ctx = ctx }
func (s *XbotService) Shutdown()                   {}

func (s *XbotService) GetServerURL() string { return backendURL }
func (s *XbotService) Ping() string         { return "xbot-gui v0.2.0" }
