package core

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/fx"
	"net/http"
	_ "net/http/pprof"
)

func init() {
	Provides(NewServerMetrics)

	promExporter, err := prometheus.New(prometheus.WithoutScopeInfo())
	if err != nil {
		panic(err)
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceVersion(BuildVersion),
		))

	if err != nil {
		panic(err)
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(promExporter),
		metric.WithResource(res),
	)

	provider.Meter("meterName")

	otel.SetMeterProvider(provider)

}

type ServerMetrics struct {
}

func NewServerMetrics(lc fx.Lifecycle) *ServerMetrics {
	s := &ServerMetrics{}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				http.Handle("/metrics", promhttp.Handler())
				http.ListenAndServe(":2112", nil)
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return nil
		},
	})
	return s
}
