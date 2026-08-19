package webui

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// Regression test for a real bug found live: a nil Go slice — the
// ordinary zero value every ListXxx function in this codebase returns for
// "found nothing" — marshals to JSON `null`, which crashed the
// dashboard's `repos.reduce(...)` in the browser the moment an account
// with zero repositories loaded it. writeJSONList is the fix; this locks
// in that a nil slice always serializes as `[]`, never `null`.
func TestWriteJSONListNeverSerializesNull(t *testing.T) {
	var nilRepos []string // deliberately nil, not just empty
	w := httptest.NewRecorder()
	writeJSONList(w, 200, nilRepos)

	body := w.Body.String()
	if body != "[]\n" {
		t.Errorf("writeJSONList(nil) wrote %q, want \"[]\\n\"", body)
	}

	// And a real value still round-trips correctly.
	var decoded []string
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("output did not parse as JSON: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("decoded %d items, want 0", len(decoded))
	}
}

func TestWriteJSONListPreservesRealItems(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSONList(w, 200, []int{1, 2, 3})

	var decoded []int
	if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("output did not parse as JSON: %v", err)
	}
	if len(decoded) != 3 || decoded[0] != 1 || decoded[2] != 3 {
		t.Errorf("decoded %v, want [1 2 3]", decoded)
	}
}
