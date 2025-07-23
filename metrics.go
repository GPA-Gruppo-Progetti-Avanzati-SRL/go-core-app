package core

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"

	"go.uber.org/fx"
	"net/http"
	_ "net/http/pprof"
)

func init() {

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

	provider.Meter(AppName)

	otel.SetMeterProvider(provider)

}

type ServerMetrics struct {
}

func NewServerMetrics(lc fx.Lifecycle) *ServerMetrics {
	s := &ServerMetrics{}
	srv := http.NewServeMux()
	srv.Handle("/metrics", promhttp.Handler())
	server := &http.Server{Addr: ":2112", Handler: srv}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go func() {
				log.Info().Msg("Starting metrics server on port 2112")
				if err := server.ListenAndServe(); err != nil {

				}

			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info().Msg("Shutting down server metrics")
			if err := server.Shutdown(ctx); err != nil {
				return err
			}
			return nil
		},
	})
	return s
}
