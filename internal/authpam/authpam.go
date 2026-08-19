// Package authpam authenticates a username/password pair against the host's
// real PAM stack — the same mechanism `login` and `sshd` use, and the reason
// an ngxborg tenant's web UI password is simply their POSIX account
// password, never a separate hash this application stores itself.
//
// This talks to libpam directly via cgo rather than shelling out to a
// setuid helper or vendoring a third-party PAM binding, matching this
// project's preference for minimal, self-contained code. The real cost of
// that choice: this package needs libpam0g-dev's headers to build
// (CGO_ENABLED=1, -lpam) and links dynamically against libpam at runtime,
// so — unlike ngxsetup, which cross-compiles to a static binary from any
// machine — ngxborg has to be built on (or for) a real Linux host with PAM
// development headers installed, and cannot be built on macOS at all: PAM
// is a Linux/BSD concept with no equivalent here. See `ngxborg doctor` and
// the README for the exact package to install.
package authpam

/*
#cgo LDFLAGS: -lpam
#include <security/pam_appl.h>
#include <stdlib.h>
#include <string.h>

// conv_func answers every prompt PAM's auth module raises with the one
// password the caller supplied, regardless of the message style. This is
// correct for pam_unix.so's ordinary "Password: " prompt, which is the only
// module ServiceName's PAM stack is configured with (see
// internal/provision's setup step) — a stack that asked more than one
// question (a second factor, a security question) would need a real,
// message-aware conversation, which this deliberately does not attempt to
// be: ngxborg's job is "is this the account's password", not general PAM
// conversation handling.
static int conv_func(int num_msg, const struct pam_message **msg,
                      struct pam_response **resp, void *appdata_ptr) {
	struct pam_response *reply = calloc((size_t)num_msg, sizeof(struct pam_response));
	if (reply == NULL) {
		return PAM_BUF_ERR;
	}
	for (int i = 0; i < num_msg; i++) {
		switch (msg[i]->msg_style) {
		case PAM_PROMPT_ECHO_OFF:
		case PAM_PROMPT_ECHO_ON:
			reply[i].resp = strdup((const char *)appdata_ptr);
			reply[i].resp_retcode = 0;
			break;
		default:
			reply[i].resp = NULL;
			reply[i].resp_retcode = 0;
			break;
		}
	}
	*resp = reply;
	return PAM_SUCCESS;
}

// authenticate runs the standard two-step PAM login check: pam_authenticate
// confirms the password, pam_acct_mgmt confirms the account itself is
// currently usable (not locked, not expired) — a distinction that matters
// here specifically, since posix.CreateUser deliberately leaves a freshly
// created account password-locked, and a correct-but-irrelevant password
// must not authenticate against a locked account.
static int authenticate(const char *service, const char *username, const char *password, char **err_out) {
	struct pam_conv conv;
	conv.conv = conv_func;
	conv.appdata_ptr = (void *)password;

	pam_handle_t *pamh = NULL;
	int rc = pam_start(service, username, &conv, &pamh);
	if (rc != PAM_SUCCESS) {
		// pamh is not yet valid here; Linux-PAM's pam_strerror does not
		// dereference its first argument, so NULL is safe on the platform
		// this package targets.
		*err_out = strdup(pam_strerror(NULL, rc));
		return rc;
	}

	rc = pam_authenticate(pamh, 0);
	if (rc == PAM_SUCCESS) {
		rc = pam_acct_mgmt(pamh, 0);
	}
	if (rc != PAM_SUCCESS) {
		*err_out = strdup(pam_strerror(pamh, rc));
	}

	pam_end(pamh, rc);
	return rc;
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

// ServiceName is the PAM service ngxborg authenticates under — the name of
// the stack file `ngxborg setup` writes to /etc/pam.d/. A username/password
// pair is checked against whatever that file configures, not against PAM's
// system-wide default, so ngxborg's login policy can be tightened
// (requiring a second factor module, for instance) without touching every
// other service on the box that also happens to use PAM.
const ServiceName = "ngxborg"

// Authenticate checks a username/password pair against the host's PAM
// stack for ServiceName. A nil return means both the password was correct
// and the account is currently allowed to log in; any error means the
// login must be refused — the error text (PAM's own message, e.g.
// "Authentication failure" or "Account locked") is safe to show to an
// operator but deliberately vague enough that it should not be shown
// verbatim to whoever is attempting to log in, the same reasoning any
// login form applies to "wrong username or wrong password."
func Authenticate(username, password string) error {
	cService := C.CString(ServiceName)
	defer C.free(unsafe.Pointer(cService))
	cUsername := C.CString(username)
	defer C.free(unsafe.Pointer(cUsername))
	cPassword := C.CString(password)
	defer C.free(unsafe.Pointer(cPassword))

	var cErr *C.char
	rc := C.authenticate(cService, cUsername, cPassword, &cErr)
	if rc != 0 {
		msg := "authentication failed"
		if cErr != nil {
			msg = C.GoString(cErr)
			C.free(unsafe.Pointer(cErr))
		}
		return errors.New(msg)
	}
	return nil
}
