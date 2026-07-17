package core

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// init imposta GOMEMLIMIT dal limite del cgroup, ma SOLO se il processo gira
// effettivamente dentro un cgroup con un limite di memoria (tipicamente in
// container/k8s). Fuori da un cgroup — o su sistemi che non li supportano, es.
// macOS in sviluppo locale — la libreria non deve emettere l'errore
// "failed to set GOMEMLIMIT: cgroups is not supported on this system".
// Per ottenerlo avvolgiamo FromCgroup convertendo "non in un cgroup" in
// memlimit.ErrNoLimit, che il framework tratta come skip silenzioso.
//
// Tutto il logging della libreria è instradato su zerolog a livello trace,
// così non appare al livello di log di default.
func init() {
	memlimit.SetGoMemLimitWithOpts(
		memlimit.WithProvider(cgroupOnlyProvider),
		memlimit.WithLogger(slog.New(zerologTraceHandler{logger: log.Logger})),
	)
}

// cgroupOnlyProvider ritorna il limite del cgroup; se il processo non è dentro
// un cgroup (ErrNoCgroup) o il sistema non li supporta (ErrCgroupsNotSupported)
// ritorna memlimit.ErrNoLimit invece di propagare l'errore.
func cgroupOnlyProvider() (uint64, error) {
	limit, err := memlimit.FromCgroup()
	if errors.Is(err, memlimit.ErrNoCgroup) || errors.Is(err, memlimit.ErrCgroupsNotSupported) {
		return 0, memlimit.ErrNoLimit
	}
	return limit, err
}

// zerologTraceHandler è uno slog.Handler che inoltra ogni record a zerolog
// SEMPRE a livello trace, indipendentemente dal livello slog originale.
type zerologTraceHandler struct {
	logger zerolog.Logger
	attrs  []slog.Attr
}

func (h zerologTraceHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return h.logger.GetLevel() <= zerolog.TraceLevel
}

func (h zerologTraceHandler) Handle(_ context.Context, r slog.Record) error {
	e := h.logger.Trace()
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

func (h zerologTraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nh := h
	nh.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return nh
}

func (h zerologTraceHandler) WithGroup(_ string) slog.Handler {
	// memlimit non usa gruppi; li ignoriamo mantenendo gli attributi.
	return h
}
