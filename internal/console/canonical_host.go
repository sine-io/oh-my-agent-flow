package console

import (
	"net"
	"net/http"
	"strings"
)

// RedirectLocalhostTo127 returns a handler that redirects UI GET/HEAD requests
// from http://localhost:<port> to http://127.0.0.1:<port>.
//
// canonicalHostPort must be in the form "127.0.0.1:<port>".
func RedirectLocalhostTo127(canonicalHostPort string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			if hostWithoutPort(r.Host) == "localhost" {
				target := "http://" + canonicalHostPort + r.URL.RequestURI()
				http.Redirect(w, r, target, http.StatusFound)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func hostWithoutPort(hostport string) string {
	if hostport == "" {
		return ""
	}
	if strings.Contains(hostport, ":") {
		host, _, err := net.SplitHostPort(hostport)
		if err == nil {
			return host
		}
	}
	return hostport
}
