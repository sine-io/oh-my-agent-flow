package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sine-io/oh-my-agent-flow/internal/console"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	port := flag.Int("port", 0, "Port to bind (0 = random free port)")
	noOpen := flag.Bool("no-open", false, "Disable auto-opening the browser")
	flag.Parse()

	listener, baseURL, err := console.ListenLocal(*port)
	if err != nil {
		log.Fatalf("startup error: %v", err)
	}

	actualPort := listener.Addr().(*net.TCPAddr).Port
	canonicalHostPort := fmt.Sprintf("127.0.0.1:%d", actualPort)
	baseOrigin127 := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
	baseOriginLocalhost := fmt.Sprintf("http://localhost:%d", actualPort)

	sessionToken, err := console.GenerateSessionToken()
	if err != nil {
		log.Fatalf("startup error: %v", err)
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		log.Fatalf("startup error: %v", err)
	}
	fsReader, err := console.NewFSReader(console.FSReadConfig{
		ProjectRoot: projectRoot,
		MaxBytes:    console.DefaultMaxReadBytes,
	})
	if err != nil {
		log.Fatalf("startup error: %v", err)
	}

	streamHub := console.NewStreamHub(console.StreamHubConfig{
		ArchiveDir: filepath.Join(projectRoot, ".ohmyagentflow", "runs"),
	})

	fireSvc, err := console.NewFireService(console.FireConfig{
		ProjectRoot: projectRoot,
		Hub:         streamHub,
	})
	if err != nil {
		log.Fatalf("startup error: %v", err)
	}

	prdChat, err := console.NewPRDChatService(console.PRDChatConfig{
		ProjectRoot: projectRoot,
		SessionTTL:  30 * time.Minute,
	})
	if err != nil {
		log.Fatalf("startup error: %v", err)
	}

	fmt.Println(baseURL)

	if !*noOpen {
		if err := tryAutoOpen(baseURL); err != nil {
			log.Printf("warning: failed to auto-open browser: %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		htmlBytes, err := console.RenderIndexHTML(console.IndexPageData{
			SessionToken: sessionToken,
			ProjectRoot:  projectRoot,
		})
		if err != nil {
			log.Printf("warning: failed to render index page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(htmlBytes)
	})

	mux.HandleFunc("GET /api/fs/read", console.FSReadHandler(fsReader))
	mux.HandleFunc("GET /api/stream", console.StreamHandler(streamHub))

	mux.HandleFunc("POST /api/init", console.InitHandler(console.InitConfig{ProjectRoot: projectRoot}))
	mux.HandleFunc("POST /api/prd/generate", console.PRDGenerateHandler(console.PRDGenerateConfig{ProjectRoot: projectRoot}))
	mux.HandleFunc("POST /api/prd/chat/session", prdChat.SessionHandler())
	mux.HandleFunc("POST /api/prd/chat/message", prdChat.MessageHandler())
	mux.HandleFunc("GET /api/prd/chat/state", prdChat.StateHandler())
	mux.HandleFunc("POST /api/prd/chat/finalize", prdChat.FinalizeHandler())
	mux.HandleFunc("POST /api/convert", console.ConvertHandler(console.ConvertConfig{ProjectRoot: projectRoot, FSReader: fsReader}))
	mux.HandleFunc("POST /api/fire", fireSvc.StartHandler())
	mux.HandleFunc("POST /api/fire/stop", fireSvc.StopHandler())

	mux.HandleFunc("POST /api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})

	protected := console.RequireWriteAuth(mux, console.WriteAuthConfig{
		SessionToken:   sessionToken,
		AllowedOrigins: []string{baseOrigin127, baseOriginLocalhost},
	})

	server := &http.Server{Handler: console.RedirectLocalhostTo127(canonicalHostPort, protected)}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

func tryAutoOpen(url string) error {
	var cmdName string
	switch runtime.GOOS {
	case "darwin":
		cmdName = "open"
	case "linux":
		cmdName = "xdg-open"
	default:
		return fmt.Errorf("unsupported OS for auto-open: %s", runtime.GOOS)
	}

	cmd := exec.Command(cmdName, url)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		_ = cmd.Wait()
	}()
	return nil
}
