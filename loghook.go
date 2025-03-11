package core

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var logOtelMeter = otel.Meter("logMeter")

// LevelHook applies a different hook for each level.
type MetricLogHook struct {
	LogEvent metric.Int64Counter
}

func (m *MetricLogHook) Init() {

	m.LogEvent, _ = logOtelMeter.Int64Counter("log.events", metric.WithUnit("{events}"), metric.WithDescription("Total number of log events"))
}
func (m *MetricLogHook) Run(e *zerolog.Event, level zerolog.Level, message string) {

	m.LogEvent.Add(context.Background(), 1, metric.WithAttributeSet(attribute.NewSet(attribute.String("level", level.String()))))

}
