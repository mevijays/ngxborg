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
)

//go:embed static
var staticFS embed.FS

type server struct {
	sessions *sessionStore
}

// Serve blocks, running the web UI until ctx is cancelled or the listener
// fails.
func Serve(ctx context.Context, addr string) error {
	cert, err := ensureCert()
	if err != nil {
		return fmt.Errorf("preparing TLS certificate: %w", err)
	}

	s := &server{sessions: newSessionStore()}
	mux := http.NewServeMux()
	s.routes(mux)

	httpServer := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}

	go func() {
		<-ctx.Done()
		httpServer.Close()
	}()

	logx.Info("ngxborg web UI listening on https://%s", addr)
	err = httpServer.ListenAndServeTLS("", "")
	if err == http.ErrServerClosed {
		return nil
	}
	return err
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
}
