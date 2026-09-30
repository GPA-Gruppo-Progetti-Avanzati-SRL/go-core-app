package observability

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

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
