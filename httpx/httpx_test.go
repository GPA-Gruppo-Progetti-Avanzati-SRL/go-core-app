package httpx

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func TestHttpClient_HaUnTimeout(t *testing.T) {
	if c := GenerateHttpClientWithInstrumentation("svc"); c.Timeout != DefaultHttpClientTimeout {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
	if c := GenerateHttpClientWithInstrumentation("svc", time.Second); c.Timeout != time.Second {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
}

func TestServeOnLifecycle_PortaOccupata(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	lc := fxtest.NewLifecycle(t)
	ServeOnLifecycle(lc, nopShutdowner{}, &http.Server{Addr: busy.Addr().String()}, "test")
	if err := lc.Start(context.Background()); err == nil {
		_ = lc.Stop(context.Background())
		t.Fatal("OnStart doveva fallire su porta occupata")
	}
}

type nopShutdowner struct{}

func (nopShutdowner) Shutdown(...fx.ShutdownOption) error { return nil }

// recShutdowner registra le opzioni con cui è stato chiesto lo shutdown.
type recShutdowner struct{ got chan []fx.ShutdownOption }

func (r recShutdowner) Shutdown(opts ...fx.ShutdownOption) error { r.got <- opts; return nil }

// TestServeOnLifecycle_ServerMortoEsceConCodice1: se l'accept loop muore dopo l'avvio — qui il
// listener chiuso da sotto — il processo deve uscire con codice 1, non restare vivo senza servire
// nulla. Un arresto ordinato (Shutdown dell'OnStop) invece non chiede nessuno shutdown.
func TestServeOnLifecycle_ServerMortoEsceConCodice1(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sh := recShutdowner{got: make(chan []fx.ShutdownOption, 1)}
	lc := fxtest.NewLifecycle(t)
	serveOnLifecycle(lc, sh, &http.Server{Addr: ln.Addr().String()}, "test", func() (net.Listener, error) { return ln, nil })
	lc.RequireStart()

	_ = ln.Close()
	select {
	case opts := <-sh.got:
		if len(opts) != 1 || opts[0] != fx.ExitCode(1) {
			t.Fatalf("Shutdown chiamato con %v, atteso fx.ExitCode(1)", opts)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("il server è morto ma nessuno ha chiesto lo shutdown dell'applicazione")
	}
	lc.RequireStop()
}

func TestServeOnLifecycle_StopOrdinatoNonChiedeShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sh := recShutdowner{got: make(chan []fx.ShutdownOption, 1)}
	lc := fxtest.NewLifecycle(t)
	serveOnLifecycle(lc, sh, &http.Server{Addr: ln.Addr().String()}, "test", func() (net.Listener, error) { return ln, nil })
	lc.RequireStart()
	lc.RequireStop()
	select {
	case opts := <-sh.got:
		t.Fatalf("arresto ordinato: shutdown non richiesto, invece %v", opts)
	case <-time.After(100 * time.Millisecond):
	}
}
