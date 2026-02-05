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

	actualPort := listener.Addr().(*net.TCPAddr).Port
	canonicalHostPort := fmt.Sprintf("127.0.0.1:%d", actualPort)
	baseOrigin127 := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
	baseOriginLocalhost := fmt.Sprintf("http://localhost:%d", actualPort)

	sessionToken, err := console.GenerateSessionToken()
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
    <meta name="ohmyagentflow-session-token" content="` + sessionToken + `" />
    <title>Oh My Agent Flow</title>
    <script>
      (function () {
        const meta = document.querySelector('meta[name="ohmyagentflow-session-token"]');
        const token = meta && meta.content ? meta.content : '';
        if (!token) return;

        const originalFetch = window.fetch.bind(window);
        window.fetch = function (input, init) {
          const requestInit = init || {};
          const method = (requestInit.method || (input instanceof Request ? input.method : 'GET') || 'GET').toUpperCase();
          const url = new URL(input instanceof Request ? input.url : String(input), window.location.href);
          if (method !== 'POST' || !url.pathname.startsWith('/api/')) {
            return originalFetch(input, requestInit);
          }

          if (input instanceof Request) {
            const headers = new Headers(input.headers);
            if (requestInit.headers) new Headers(requestInit.headers).forEach((v, k) => headers.set(k, v));
            headers.set('X-Session-Token', token);
            const nextRequest = new Request(input, Object.assign({}, requestInit, { headers }));
            return originalFetch(nextRequest);
          }

          const headers = new Headers(requestInit.headers || {});
          headers.set('X-Session-Token', token);
          return originalFetch(input, Object.assign({}, requestInit, { headers }));
        };
      })();
    </script>
  </head>
  <body>
    <h1>Oh My Agent Flow</h1>
    <p>Console server is running.</p>
  </body>
</html>
`))
	})

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
