package observability

import (
	"context"
	"fmt"
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
	group string // prefisso dei gruppi aperti con WithGroup, "" o "a.b."
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
		e = addAttr(e, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		e = addAttr(e, h.group, a)
		return true
	})
	e.Msg(r.Message)
	return nil
}

// addAttr scrive un attributo slog nel tipo zerolog corrispondente. Passare tutto da
// Interface(key, a.Value.Any()) serializzava in JSON anche gli error — e un errors.errorString in
// JSON è un oggetto vuoto: ogni errore di gocron arrivava nei log come `"error":{}`.
// I LogValuer si risolvono, i gruppi diventano prefissi `gruppo.chiave`, gli attributi vuoti si
// saltano (è la regola di slog).
func addAttr(e *zerolog.Event, prefix string, a slog.Attr) *zerolog.Event {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return e
	}
	key := prefix + a.Key
	switch a.Value.Kind() {
	case slog.KindGroup:
		p := prefix
		if a.Key != "" {
			p = key + "."
		}
		for _, ga := range a.Value.Group() {
			e = addAttr(e, p, ga)
		}
		return e
	case slog.KindString:
		return e.Str(key, a.Value.String())
	case slog.KindInt64:
		return e.Int64(key, a.Value.Int64())
	case slog.KindUint64:
		return e.Uint64(key, a.Value.Uint64())
	case slog.KindFloat64:
		return e.Float64(key, a.Value.Float64())
	case slog.KindBool:
		return e.Bool(key, a.Value.Bool())
	case slog.KindDuration:
		return e.Dur(key, a.Value.Duration())
	case slog.KindTime:
		return e.Time(key, a.Value.Time())
	}
	switch v := a.Value.Any().(type) {
	case error:
		return e.AnErr(key, v)
	case fmt.Stringer:
		return e.Stringer(key, v)
	default:
		return e.Interface(key, v)
	}
}

func (h zerologHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := h
	nh.attrs = append([]slog.Attr{}, h.attrs...)
	for _, a := range attrs {
		// La chiave prende il gruppo aperto adesso: un WithGroup successivo non la sposta.
		if h.group != "" {
			a.Key = h.group + a.Key
		}
		nh.attrs = append(nh.attrs, a)
	}
	return nh
}

// WithGroup apre un gruppo: le chiavi degli attributi successivi diventano `gruppo.chiave`, così
// due gruppi con una chiave omonima non si sovrascrivono.
func (h zerologHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	nh := h
	nh.group = h.group + name + "."
	return nh
}
