package observability

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

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

type secret string

func (secret) LogValue() slog.Value { return slog.StringValue("[redacted]") }

// TestSlogHandler_TipiEGruppi: gli attributi arrivano a zerolog col loro tipo. Prima passavano
// tutti da Interface, cioè da json.Marshal, e un error diventava `{}`: ogni errore di gocron
// finiva nei log senza messaggio.
func TestSlogHandler_TipiEGruppi(t *testing.T) {
	prevLogger, prevLevel := log.Logger, zerolog.GlobalLevel()
	t.Cleanup(func() { log.Logger = prevLogger; zerolog.SetGlobalLevel(prevLevel) })
	var buf bytes.Buffer
	log.Logger = zerolog.New(&buf)
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	l := slog.New(SlogHandler("gocron")).With("job", "import")
	l.WithGroup("lock").Error("fallito",
		"error", errors.New("boom"),
		"tries", 3,
		"wait", 1500*time.Millisecond,
		"pwd", secret("x"),
		slog.Group("key", "name", "k1"))

	out := buf.String()
	for _, want := range []string{
		`"lock.error":"boom"`, `"job":"import"`, `"lock.tries":3`, `"lock.pwd":"[redacted]"`,
		`"lock.key.name":"k1"`, `"lock.wait":1500`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("manca %s in %s", want, out)
		}
	}
	if strings.Contains(out, `{}`) {
		t.Errorf("un attributo è stato serializzato vuoto: %s", out)
	}
}
