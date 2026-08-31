package core

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProfilingHandler_EsponeIProfiliDocumentati verifica ciò che la documentazione promette.
// §4.6 dell'analisi non era un bug di codice ma una divergenza fra doc e comportamento: il claim
// su goroutineleak è qui perché è quello che CLAUDE.md e i README nominano.
func TestProfilingHandler_EsponeIProfiliDocumentati(t *testing.T) {
	srv := httptest.NewServer(ProfilingHandler())
	defer srv.Close()

	for _, path := range []string{
		"/debug/pprof/",
		"/debug/pprof/goroutineleak?debug=1",
		"/debug/pprof/goroutine?debug=1",
		"/debug/pprof/heap?debug=1",
		"/debug/pprof/cmdline",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, atteso 200 (body: %.200s)", path, resp.StatusCode, body)
		}
	}
}

// TestProfilingHandler_IndexElencaGoroutineleak: Index enumera i profili di runtime/pprof, quindi
// goroutineleak compare senza che ProfilingHandler lo nomini. Se una versione di Go lo togliesse,
// questo test lo direbbe invece di lasciare la doc a promettere un profilo inesistente.
func TestProfilingHandler_IndexElencaGoroutineleak(t *testing.T) {
	srv := httptest.NewServer(ProfilingHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/debug/pprof/")
	if err != nil {
		t.Fatalf("GET index: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), "goroutineleak") {
		t.Errorf("l'index di pprof non elenca goroutineleak; body:\n%s", body)
	}
}

// TestProfilingHandler_NonServeAltro: l'handler è montato su mux che servono anche altro, quindi
// non deve rispondere fuori dal suo prefisso.
func TestProfilingHandler_NonServeAltro(t *testing.T) {
	srv := httptest.NewServer(ProfilingHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /metrics: status %d, atteso 404", resp.StatusCode)
	}
}
