package webui

import (
	"encoding/json"
	"errors"
	"net/http"

	"ngxborg/internal/authpam"
	"ngxborg/internal/borgrepo"
	"ngxborg/internal/posix"
	"ngxborg/internal/provision"
	"ngxborg/internal/sshaccess"
	"ngxborg/internal/system"
)

var (
	errForbiddenTenant = errors.New("you can only manage your own account")
	errTenantRequired  = errors.New("tenant is required")
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeJSONList is writeJSON for slice-shaped responses specifically: a nil
// Go slice — the normal, idiomatic zero value for "found nothing", and
// what every ListXxx function in this codebase correctly returns for that
// case — marshals to JSON `null`, not `[]`. That is harmless in Go itself
// (ranging over a nil slice is a no-op, len(nil) is 0) but not in
// JavaScript: `null.reduce` or `null.map` throws, so every list endpoint
// the frontend consumes needs to guarantee an empty array, never null,
// confirmed live when the dashboard's repo count crashed on exactly this
// for a fresh account with zero repositories. Fixed once, here, rather
// than requiring every internal ListXxx function to abandon Go's own
// idiomatic nil-slice convention, and rather than requiring every call
// site in app.js to defensively guard against a null the backend should
// never have sent in the first place — this is the one seam where a Go
// value becomes a value something else has to trust.
func writeJSONList[T any](w http.ResponseWriter, status int, items []T) {
	if items == nil {
		items = []T{}
	}
	writeJSON(w, status, items)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// ---- auth -------------------------------------------------------------

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct{ Username, Password string }
	if err := readJSON(r, &req); err != nil || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if !posix.IsTenant(req.Username) {
		// Deliberately the same error PAM failure gets below: whether the
		// account exists at all is not information a login form should
		// leak.
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := authpam.Authenticate(req.Username, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	admin := posix.IsAdmin(req.Username)
	token, err := s.sessions.create(req.Username, admin)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not start a session")
		return
	}
	setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"username": req.Username, "admin": admin})
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(cookieName); err == nil {
		s.sessions.destroy(cookie.Value)
	}
	clearSessionCookie(w)
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	writeJSON(w, http.StatusOK, map[string]any{"username": sess.username, "admin": sess.admin})
}

// ---- repositories -------------------------------------------------------

func (s *server) handleReposList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenantParam := r.URL.Query().Get("tenant")

	var repos []borgrepo.Repo
	var err error
	switch {
	case !sess.admin:
		tenant, serr := scopeTenant(sess, tenantParam)
		if serr != nil {
			writeError(w, http.StatusForbidden, serr.Error())
			return
		}
		repos, err = borgrepo.ListRepos(tenant)
	case tenantParam != "":
		repos, err = borgrepo.ListRepos(tenantParam)
	default:
		repos, err = borgrepo.ListAllRepos()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONList(w, http.StatusOK, repos)
}

func (s *server) handleReposCreate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var req struct{ Tenant, Name string }
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tenant, err := scopeTenant(sess, req.Tenant)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	repo, err := borgrepo.CreateRepo(tenant, req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, repo)
}

func (s *server) handleReposDelete(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenant, err := scopeTenant(sess, r.PathValue("tenant"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := borgrepo.Delete(tenant, r.PathValue("name")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleReposPurge(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenant, err := scopeTenant(sess, r.PathValue("tenant"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := borgrepo.Purge(tenant, r.PathValue("name")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleReposDisable(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenant, err := scopeTenant(sess, r.PathValue("tenant"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := borgrepo.Disable(tenant, r.PathValue("name")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleReposEnable(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenant, err := scopeTenant(sess, r.PathValue("tenant"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := borgrepo.Enable(tenant, r.PathValue("name")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ---- ssh keys -------------------------------------------------------------

func (s *server) handleKeysList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenant, err := scopeTenant(sess, r.URL.Query().Get("tenant"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	keys, err := sshaccess.ListKeys(tenant)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONList(w, http.StatusOK, keys)
}

func (s *server) handleKeysAdd(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var req struct {
		Tenant, Repo, PublicKey string
		AppendOnly              bool
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tenant, err := scopeTenant(sess, req.Tenant)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if !borgrepo.Exists(tenant, req.Repo) {
		writeError(w, http.StatusBadRequest, "no such repository — create it first")
		return
	}
	if err := sshaccess.AddKey(tenant, borgrepo.Path(tenant, req.Repo), req.PublicKey, req.AppendOnly); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *server) handleKeysRemove(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	tenant, err := scopeTenant(sess, r.PathValue("tenant"))
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if err := sshaccess.RemoveKey(tenant, r.PathValue("material")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ---- users (admin only) ----------------------------------------------------

func (s *server) handleUsersList(w http.ResponseWriter, r *http.Request) {
	names, err := posix.ListTenants()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	type userInfo struct {
		Username string `json:"username"`
		Admin    bool   `json:"admin"`
		Keys     int    `json:"keys"`
		Disabled bool   `json:"disabled"`
	}
	var out []userInfo
	for _, name := range names {
		keys, _ := sshaccess.ListKeys(name)
		disabled, _ := posix.IsDisabled(name)
		out = append(out, userInfo{Username: name, Admin: posix.IsAdmin(name), Keys: len(keys), Disabled: disabled})
	}
	writeJSONList(w, http.StatusOK, out)
}

func (s *server) handleUsersCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string
		Admin    bool
	}
	if err := readJSON(r, &req); err != nil || req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if err := posix.CreateUser(r.Context(), system.Runner{}, req.Username, req.Admin); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *server) handleUsersDelete(w http.ResponseWriter, r *http.Request) {
	if err := posix.DeleteUser(r.Context(), system.Runner{}, r.PathValue("username")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleUsersDisable and handleUsersEnable are admin-only (see server.go's
// routing, not a self-service scopeTenant call like the repo/key
// endpoints): an account disabling itself through the same session it is
// about to lock out has no recovery path back through this UI, so that
// choice is left to a different admin account instead.
func (s *server) handleUsersDisable(w http.ResponseWriter, r *http.Request) {
	if err := posix.Disable(r.Context(), system.Runner{}, r.PathValue("username")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *server) handleUsersEnable(w http.ResponseWriter, r *http.Request) {
	if err := posix.Enable(r.Context(), system.Runner{}, r.PathValue("username")); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePasswd sets or resets a login password — the same operation
// `ngxborg user passwd` performs, and the fix for a real gap found in
// testing: the web UI could create an account but had no way at all to
// give it a usable password afterward. A blank password in the request
// means "generate one", mirroring the CLI's --generate flag and
// ngxsetup's own established pattern for every other generated secret —
// shown back to the caller exactly once in the response, never logged,
// never retrievable again after this call returns.
func (s *server) handlePasswd(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r)
	var req struct{ Username, Password string }
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target := req.Username
	if target == "" {
		target = sess.username
	}
	if !sess.admin && target != sess.username {
		writeError(w, http.StatusForbidden, errForbiddenTenant.Error())
		return
	}

	password := req.Password
	generated := ""
	if password == "" {
		p, err := system.Password(20)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not generate a password")
			return
		}
		password, generated = p, p
	}
	if err := posix.SetPassword(r.Context(), system.Runner{}, target, password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"generated_password": generated})
}

// ---- doctor (admin only) ----------------------------------------------------

func (s *server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	c, err := provision.New(r.Context(), false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSONList(w, http.StatusOK, c.Diagnose())
}
