package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/snarktank/oh-my-agent-flow/internal/console"
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

	fmt.Println(baseURL)

	if !*noOpen {
		if err := tryAutoOpen(baseURL); err != nil {
			log.Printf("warning: failed to auto-open browser: %v", err)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <title>Oh My Agent Flow</title>
  </head>
  <body>
    <h1>Oh My Agent Flow</h1>
    <p>Console server is running.</p>
  </body>
</html>
`))
	})

	server := &http.Server{Handler: mux}
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
