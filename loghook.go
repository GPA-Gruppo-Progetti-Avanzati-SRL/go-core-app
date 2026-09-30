package core

import (
	"context"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var logOtelMeter = otel.Meter("logMeter")

// MetricLogHook conta gli eventi di log per livello (counter `log.events{level}`).
type MetricLogHook struct {
	LogEvent metric.Int64Counter

	// byLevel è l'opzione di misura già costruita per ogni livello zerolog (da TraceLevel a
	// PanicLevel): Run gira su ogni evento di log, e costruire lì attribute.NewSet significava
	// un'allocazione per riga di log.
	byLevel [8]metric.AddOption
	other   metric.AddOption
}

func (m *MetricLogHook) Init() {
	counter, err := logOtelMeter.Int64Counter("log.events", metric.WithUnit("{events}"), metric.WithDescription("Total number of log events"))
	if err != nil {
		// L'hook resta installabile: un Int64Counter che fallisce ritorna comunque un counter
		// no-op, quindi la metrica manca ma il logging no. Scartarlo in silenzio lasciava
		// l'assenza della metrica senza spiegazione.
		log.Warn().Err(err).Msg("log.events counter not available")
	}
	m.LogEvent = counter
	for i := range m.byLevel {
		lvl := zerolog.Level(i) + zerolog.TraceLevel
		m.byLevel[i] = levelOption(lvl)
	}
	m.other = levelOption(zerolog.NoLevel)
}

func levelOption(l zerolog.Level) metric.AddOption {
	return metric.WithAttributeSet(attribute.NewSet(attribute.String("level", l.String())))
}

func (m *MetricLogHook) Run(e *zerolog.Event, level zerolog.Level, message string) {
	if m.LogEvent == nil {
		return
	}
	opt := m.other
	if i := int(level - zerolog.TraceLevel); i >= 0 && i < len(m.byLevel) && m.byLevel[i] != nil {
		opt = m.byLevel[i]
	}
	m.LogEvent.Add(context.Background(), 1, opt)
}

// metricLogHook è l'unico hook installato da ReadConfig: il counter si registra una volta sola
// anche se la config viene riletta.
var metricLogHook = sync.OnceValue(func() *MetricLogHook {
	h := &MetricLogHook{}
	h.Init()
	return h
})
