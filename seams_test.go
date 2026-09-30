package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx/fxtest"
)

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

func TestErrors_AmbitoECodice(t *testing.T) {
	lib := Errors{Ambit: "go-core-test"}
	cause := errors.New("boom")
	for _, e := range []*ApplicationError{lib.Tech("T-1").WithCause(cause), lib.Business("B-1").WithCause(cause)} {
		if e.Ambit != "go-core-test" || !errors.Is(e, cause) {
			t.Fatalf("errore %+v: ambito o causa persi", e)
		}
	}
	if e := lib.Tech("T-1"); e.StatusCode != 500 || e.Code != "T-1" {
		t.Fatalf("Tech = %+v", e)
	}
	if e := lib.Business("B-1"); e.StatusCode != 422 || e.Code != "B-1" {
		t.Fatalf("Business = %+v", e)
	}
	if e := lib.NotFound(); e.StatusCode != 404 || e.Ambit != "go-core-test" {
		t.Fatalf("NotFound = %+v", e)
	}
}

func TestSlogHandler_LivelliEComponente(t *testing.T) {
	prevLogger, prevLevel := log.Logger, zerolog.GlobalLevel()
	t.Cleanup(func() { log.Logger = prevLogger; zerolog.SetGlobalLevel(prevLevel) })
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf) // letto a ogni record: l'handler è creato prima, come in un init
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	l := slog.New(SlogHandler("gocron"))
	l.Debug("scartata")
	l.Warn("tenuta", "job", "x")

	out := buf.String()
	if strings.Contains(out, "scartata") {
		t.Fatalf("Debug sotto il livello globale è stata scritta: %s", out)
	}
	for _, want := range []string{`"level":"warn"`, `"component":"gocron"`, `"job":"x"`, `"message":"tenuta"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("manca %s in %s", want, out)
		}
	}
}

func TestGetHostname_MaiVuoto(t *testing.T) {
	if h := GetHostname(); h == "" || h != GetHostname() {
		t.Fatalf("GetHostname = %q", h)
	}
}

func TestTaggedFields(t *testing.T) {
	type filtro struct {
		interno string // non esportato e senza tag: prima faceva panicare go-core-mongo
		Nome    string `col:"name" op:"="`
		Eta     int    `col:"age" op:">" omitempty:""`
		Libero  string
	}
	got, err := TaggedFields(&filtro{interno: "x", Nome: "ada"}, "col", "op")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != "name" || got[0].Op != "=" || got[0].Value != "ada" {
		t.Fatalf("TaggedFields = %+v", got)
	}

	type malTaggato struct {
		nome string `col:"name" op:"="`
	}
	if _, err := TaggedFields(malTaggato{nome: "x"}, "col", "op"); err == nil {
		t.Fatal("campo non esportato con i tag: atteso errore, non panic")
	}
	var nilPtr *filtro
	for _, in := range []any{nil, nilPtr, 3} {
		if _, err := TaggedFields(in, "col", "op"); err == nil {
			t.Fatalf("TaggedFields(%v): atteso errore", in)
		}
	}
}

func TestEncryptDecrypt_RoundTripViaHex(t *testing.T) {
	ct, err := Encrypt([]byte("segreto"), "app-id")
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(hex.EncodeToString(ct), "app-id")
	if err != nil || string(pt) != "segreto" {
		t.Fatalf("round-trip = %q, %v", pt, err)
	}
	if _, err := Decrypt(hex.EncodeToString(ct), "altra-app"); err == nil {
		t.Fatal("chiave diversa: atteso errore")
	}
}
