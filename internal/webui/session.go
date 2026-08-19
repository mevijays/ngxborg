package webui

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	cookieName = "ngxborg_session"
	sessionTTL = 8 * time.Hour
)

type session struct {
	username string
	admin    bool
	expires  time.Time
}

// sessionStore is a plain in-memory map. ngxborg-web.service is a single
// process with no clustering story, so there is nothing a shared/persisted
// session store would buy here — a restart requiring everyone to log in
// again is an acceptable, honest consequence of that, not a bug to work
// around.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
}

func newSessionStore() *sessionStore {
	s := &sessionStore{sessions: make(map[string]session)}
	go s.reap()
	return s
}

func (s *sessionStore) reap() {
	for range time.Tick(10 * time.Minute) {
		s.mu.Lock()
		now := time.Now()
		for token, sess := range s.sessions {
			if now.After(sess.expires) {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}

func (s *sessionStore) create(username string, admin bool) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	s.mu.Lock()
	s.sessions[token] = session{username: username, admin: admin, expires: time.Now().Add(sessionTTL)}
	s.mu.Unlock()
	return token, nil
}

func (s *sessionStore) lookup(token string) (session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok || time.Now().After(sess.expires) {
		return session{}, false
	}
	return sess, true
}

func (s *sessionStore) destroy(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
