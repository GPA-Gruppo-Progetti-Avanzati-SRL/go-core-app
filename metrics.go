package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"go.uber.org/fx"
)

// Default del server ops. Duplicano i viper.SetDefault di ReadConfig perché NewServerMetrics
// dev'essere corretta anche quando ReadConfig non è passata (test, o un'app che legge la config
// per conto suo): senza, un campo a zero diventerebbe la porta 0 o un host vuoto.
const (
	DefaultMetricsHost              = "0.0.0.0"
	DefaultMetricsPort              = 2112
	DefaultMetricsReadHeaderTimeout = 5 * time.Second
)

// withDefaults riempie i soli campi non valorizzati. Pprof è deliberatamente assente: false è il
// default e non esiste un "non valorizzato" da distinguere — l'esposizione si chiede, non si eredita.
func (c MetricsConfig) withDefaults() MetricsConfig {
	if c.Host == "" {
		c.Host = DefaultMetricsHost
	}
	if c.Port <= 0 {
		c.Port = DefaultMetricsPort
	}
	if c.ReadHeaderTimeout <= 0 {
		c.ReadHeaderTimeout = DefaultMetricsReadHeaderTimeout
	}
	return c
}

// NewServerMetrics espone il server ops del processo: /metrics (Prometheus), /health e — solo se
// `metrics.pprof: true` — /debug/pprof/*.
//
// Ritorna error invece di panicare: è un invoke fx, quindi l'errore ferma l'avvio dell'app, che è
// ciò che ci si aspetta da un misconfig. Il listener è aperto dentro OnStart e l'errore è
// propagato: prima l'esito di ListenAndServe finiva in un blocco vuoto, quindi una porta occupata
// era silenzio totale e il processo restava "sano" senza servire nulla.
func NewServerMetrics(lc fx.Lifecycle) error {

	promExporter, err := prometheus.New(prometheus.WithoutScopeInfo())
	if err != nil {
		return fmt.Errorf("metrics: prometheus exporter: %w", err)
	}

	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceVersion(BuildVersion),
		))

	if err != nil {
		return fmt.Errorf("metrics: resource merge: %w", err)
	}

	provider := metric.NewMeterProvider(
		metric.WithReader(promExporter),
		metric.WithResource(res),
	)

	otel.SetMeterProvider(provider)

	cfg := metricsConfig.withDefaults()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/health", HealthHandler)
	if cfg.Pprof {
		// Registrazione esplicita: il blank import di net/http/pprof registrava su
		// DefaultServeMux, che questo server non è.
		mux.Handle("/debug/pprof/", ProfilingHandler())
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Listen prima di ritornare: un bind fallito deve far fallire l'avvio, non finire
			// in una goroutine che nessuno osserva.
			ln, lerr := net.Listen("tcp", addr)
			if lerr != nil {
				return fmt.Errorf("metrics: listen on %s: %w", addr, lerr)
			}
			log.Info().Str("addr", addr).Bool("pprof", cfg.Pprof).Msg("Starting metrics server")
			go func() {
				if serr := server.Serve(ln); serr != nil && serr != http.ErrServerClosed {
					log.Error().Err(serr).Str("addr", addr).Msg("metrics server stopped")
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info().Msg("Shutting down server metrics")
			return server.Shutdown(ctx)
		},
	})
	return nil
}
