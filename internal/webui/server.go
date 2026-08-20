// Package webui is ngxborg's PAM-authenticated multi-tenant dashboard: a
// tenant logs in with their real POSIX password and sees only their own
// repositories and keys; an account in posix.AdminGroup sees and manages
// everyone's — the exact same authorization split internal/cli's identity
// resolution applies, just driven by an authenticated session instead of
// the OS process's own uid, since a web request has no uid of its own to
// ask about.
package webui

import (
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"ngxborg/internal/logx"
	"ngxborg/internal/mcpserver"
)

//go:embed static
var staticFS embed.FS

type server struct {
	sessions *sessionStore
	mcp      *mcpserver.Server
}

// Serve blocks, running the web UI until ctx is cancelled or the listener
// fails. tlsMode controls TLS behavior: "self-signed" (default), "custom",
// or "none" (plain HTTP, for use behind a reverse proxy).
func Serve(ctx context.Context, addr string, tlsMode string) error {
	return serveWithCerts(ctx, addr, tlsMode, "", "")
}

// ServeWithCerts is like Serve but accepts explicit cert/key paths (used
// when tlsMode is "custom").
func ServeWithCerts(ctx context.Context, addr, tlsMode, certPath, keyPath string) error {
	return serveWithCerts(ctx, addr, tlsMode, certPath, keyPath)
}

func serveWithCerts(ctx context.Context, addr, tlsMode, certPath, keyPath string) error {
	var httpServer *http.Server

	switch tlsMode {
	case "none":
		// Plain HTTP — no TLS. Intended for use behind a reverse proxy.
		logx.Info("ngxborg web UI listening on http://%s", addr)
		httpServer = &http.Server{Addr: addr, Handler: nil} // handler set below
	case "custom":
		if certPath == "" || keyPath == "" {
			return fmt.Errorf("--tls=custom requires --tls-cert and --tls-key")
		}
		logx.Info("ngxborg web UI listening on https://%s (custom TLS)", addr)
		httpServer = &http.Server{
			Addr:      addr,
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{}}, // loaded below
		}
	default:
		// "self-signed" — generate and persist one automatically.
		logx.Info("ngxborg web UI listening on https://%s (self-signed TLS)", addr)
		cert, err := ensureCert()
		if err != nil {
			return fmt.Errorf("preparing TLS certificate: %w", err)
		}
		httpServer = &http.Server{
			Addr:      addr,
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
		}
	}

	s := &server{
		sessions: newSessionStore(),
		mcp:      mcpserver.New(),
	}
	mux := http.NewServeMux()
	s.routes(mux)
	httpServer.Handler = mux

	switch tlsMode {
	case "none":
		go func() {
			<-ctx.Done()
			httpServer.Close()
		}()
		return httpServer.ListenAndServe()
	default:
		// TLS modes: load cert if custom, otherwise ensureCert already loaded it.
		var cert tls.Certificate
		var err error
		if tlsMode == "custom" {
			cert, err = tls.LoadX509KeyPair(certPath, keyPath)
			if err != nil {
				return fmt.Errorf("loading TLS certificate: %w", err)
			}
			httpServer.TLSConfig.Certificates = []tls.Certificate{cert}
		}
		go func() {
			<-ctx.Done()
			httpServer.Close()
		}()
		return httpServer.ListenAndServeTLS("", "")
	}
}

func (s *server) routes(mux *http.ServeMux) {
	staticContent, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err) // embed.FS is compiled in; this can only fail if the build itself is broken
	}
	mux.Handle("/", http.FileServer(http.FS(staticContent)))

	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("GET /api/me", s.withAuth(s.handleMe))

	mux.HandleFunc("GET /api/repos", s.withAuth(s.handleReposList))
	mux.HandleFunc("POST /api/repos", s.withAuth(s.handleReposCreate))
	mux.HandleFunc("DELETE /api/repos/{tenant}/{name}", s.withAuth(s.handleReposDelete))
	mux.HandleFunc("POST /api/repos/{tenant}/{name}/purge", s.withAuth(s.handleReposPurge))
	mux.HandleFunc("POST /api/repos/{tenant}/{name}/disable", s.withAuth(s.handleReposDisable))
	mux.HandleFunc("POST /api/repos/{tenant}/{name}/enable", s.withAuth(s.handleReposEnable))
	mux.HandleFunc("GET /api/repos/{tenant}/{name}/client-info", s.withAuth(s.handleRepoClientInfo))

	mux.HandleFunc("GET /api/keys", s.withAuth(s.handleKeysList))
	mux.HandleFunc("POST /api/keys", s.withAuth(s.handleKeysAdd))
	mux.HandleFunc("DELETE /api/keys/{tenant}/{material}", s.withAuth(s.handleKeysRemove))

	mux.HandleFunc("GET /api/users", s.withAuth(s.withAdmin(s.handleUsersList)))
	mux.HandleFunc("POST /api/users", s.withAuth(s.withAdmin(s.handleUsersCreate)))
	mux.HandleFunc("DELETE /api/users/{username}", s.withAuth(s.withAdmin(s.handleUsersDelete)))
	mux.HandleFunc("POST /api/users/{username}/disable", s.withAuth(s.withAdmin(s.handleUsersDisable)))
	mux.HandleFunc("POST /api/users/{username}/enable", s.withAuth(s.withAdmin(s.handleUsersEnable)))

	mux.HandleFunc("POST /api/passwd", s.withAuth(s.handlePasswd))
	mux.HandleFunc("GET /api/doctor", s.withAuth(s.withAdmin(s.handleDoctor)))

	// MCP endpoint — anonymous, no PAM authentication required.
	// Agentic tools are served at /mcp on the same HTTPS port.
	mux.Handle("POST /mcp", s.mcp.StreamableHTTPHandler("/mcp"))
	mux.Handle("GET /mcp", s.mcp.StreamableHTTPHandler("/mcp"))
}
