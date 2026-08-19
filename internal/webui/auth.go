package webui

import (
	"context"
	"net/http"
)

type ctxKey int

const sessionCtxKey ctxKey = 0

func sessionFrom(r *http.Request) session {
	sess, _ := r.Context().Value(sessionCtxKey).(session)
	return sess
}

// withAuth requires a valid session cookie, injecting the session into the
// request context for handlers to read via sessionFrom.
func (s *server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(cookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not logged in")
			return
		}
		sess, ok := s.sessions.lookup(cookie.Value)
		if !ok {
			clearSessionCookie(w)
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
		next(w, r.WithContext(ctx))
	}
}

// withAdmin further requires the session belongs to an admin account. Must
// be composed inside withAuth (withAuth(withAdmin(handler))) so a session
// is already resolved by the time this checks it.
func (s *server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !sessionFrom(r).admin {
			writeError(w, http.StatusForbidden, "admin access required")
			return
		}
		next(w, r)
	}
}

// scopeTenant is withAuth's counterpart to internal/cli's scopeTenant: a
// tenant session can only ever act on its own account, no matter what a
// request parameter claims; an admin session must name one explicitly.
func scopeTenant(sess session, requested string) (string, error) {
	if !sess.admin {
		if requested != "" && requested != sess.username {
			return "", errForbiddenTenant
		}
		return sess.username, nil
	}
	if requested == "" {
		return "", errTenantRequired
	}
	return requested, nil
}
