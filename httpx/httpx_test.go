package httpx

import (
	"context"
	"net"
	"net/http"
	"sync"
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

func TestWaitContext(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { time.Sleep(10 * time.Millisecond); wg.Done() }()
	if !WaitContext(context.Background(), &wg) {
		t.Fatal("wg svuotato: atteso true")
	}

	var stuck sync.WaitGroup
	stuck.Add(1)
	defer stuck.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if WaitContext(ctx, &stuck) {
		t.Fatal("wg appeso: atteso false alla scadenza del context")
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
