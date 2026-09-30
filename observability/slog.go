package observability

import (
	"context"
	"log/slog"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// SlogHandler ritorna uno slog.Handler che inoltra a zerolog, con il livello slog tradotto in
// quello zerolog e il campo `component` a dire da dove arriva la riga. È il ponte per le
// dipendenze che loggano con slog o con un'interfaccia modellata su slog: gocron.Logger ha la
// stessa forma di *slog.Logger, quindi
//
//	gocron.WithLogger(slog.New(observability.SlogHandler("gocron")))
//
// porta i log dello scheduler nello stesso stream, formato e livello di quelli dell'app, invece di
// spegnerli o di scriverli su stdout col logger della standard library.
//
// Il logger zerolog è letto a ogni record e non alla costruzione: ReadConfig lo sostituisce, e un
// handler creato prima (in un init) altrimenti scriverebbe per sempre col logger di default.
func SlogHandler(component string) slog.Handler {
	return zerologHandler{attrs: []slog.Attr{slog.String("component", component)}}
}

// zerologHandler è lo slog.Handler di SlogHandler. Con fixed valorizzato ignora il livello del
// record e scrive sempre a quello: è il caso di automemlimit, le cui righe sono diagnostica di
// avvio e stanno a Trace qualunque livello abbiano.
type zerologHandler struct {
	attrs []slog.Attr
	fixed *zerolog.Level
}

func (h zerologHandler) level(l slog.Level) zerolog.Level {
	if h.fixed != nil {
		return *h.fixed
	}
	switch {
	case l >= slog.LevelError:
		return zerolog.ErrorLevel
	case l >= slog.LevelWarn:
		return zerolog.WarnLevel
	case l >= slog.LevelInfo:
		return zerolog.InfoLevel
	case l >= slog.LevelDebug:
		return zerolog.DebugLevel
	default:
		return zerolog.TraceLevel
	}
}

func (h zerologHandler) Enabled(_ context.Context, l slog.Level) bool {
	lvl := h.level(l)
	return lvl >= zerolog.GlobalLevel() && lvl >= log.Logger.GetLevel()
}

func (h zerologHandler) Handle(_ context.Context, r slog.Record) error {
	e := log.Logger.WithLevel(h.level(r.Level))
	if e == nil {
		return nil
	}
	for _, a := range h.attrs {
		e = e.Interface(a.Key, a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		e = e.Interface(a.Key, a.Value.Any())
		return true
	})
	e.Msg(r.Message)
	return nil
}

func (h zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return nh
}

func (h zerologHandler) WithGroup(_ string) slog.Handler {
	// Né gocron né automemlimit usano gruppi: si ignorano, mantenendo gli attributi.
	return h
}
