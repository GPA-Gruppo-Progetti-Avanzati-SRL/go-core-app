// Package observability è l'osservabilità del processo: il server ops (/metrics, /health, pprof) e il
// MeterProvider Prometheus, il TracerProvider OTLP, il contatore degli eventi di log, il ponte
// slog→zerolog e GOMEMLIMIT dal cgroup. Il wiring lo fa core (WithServerMetrics, WithTracing,
// ReadConfig); un'app usa direttamente solo HealthHandler, ProfilingHandler e SlogHandler.
package observability

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/httpx"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/internal/hooks"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"go.uber.org/fx"
)

// MetricsConfig configura il server ops di NewServerMetrics (/metrics, /health e — solo se
// richiesto — /debug/pprof/*). La sezione `metrics:` è facoltativa: i default riproducono il
// comportamento storico (0.0.0.0:2112, pprof spento). È definita in internal/hooks perché la
// deposita core.ReadConfig, che questo package non può importare.
type MetricsConfig = hooks.MetricsConfig

// MetricsSettings ritorna la configurazione del server ops effettivamente in uso, default
// compresi: è l'unico modo, per un'app o un test, di sapere su quale indirizzo il server è stato
// messo in ascolto senza reimplementare i default.
func MetricsSettings() MetricsConfig { return withDefaults(hooks.Metrics) }

// Default del server ops. Duplicano i viper.SetDefault di ReadConfig perché NewServerMetrics
// dev'essere corretta anche quando ReadConfig non è passata (test, o un'app che legge la config
// per conto suo): senza, un campo a zero diventerebbe la porta 0 o un host vuoto.
const (
	DefaultMetricsHost              = "0.0.0.0"
	DefaultMetricsPort              = 2112
	DefaultMetricsReadHeaderTimeout = 5 * time.Second

	// metricsIdleTimeout chiude le connessioni keep-alive inattive dello scraper. Non c'è un
	// WriteTimeout, di proposito: /debug/pprof/profile?seconds=N scrive per N secondi.
	metricsIdleTimeout = 2 * time.Minute
)

// withDefaults riempie i soli campi non valorizzati. Pprof è deliberatamente assente: false è il
// default e non esiste un "non valorizzato" da distinguere — l'esposizione si chiede, non si eredita.
func withDefaults(c MetricsConfig) MetricsConfig {
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

// meterProviderOnce protegge l'inizializzazione del MeterProvider, che è **stato globale di
// processo**: l'exporter si registra sul registry Prometheus di default e otel.SetMeterProvider
// installa il provider globale. Farlo due volte non è "due server ops" ma un registry con le
// metric family duplicate, quindi uno scrape che risponde 500 — cioè il monitoraggio spento
// proprio nel processo che credeva di averlo acceso.
var (
	meterProviderOnce sync.Once
	meterProviderErr  error
)

// serviceAttributes identifica il processo nelle risorse OTel di metriche e tracce: senza
// service.name ogni serie arriva come `unknown_service`. Letta a OnStart/invoke, cioè dopo che
// Boot ha impostato AppName.
func serviceAttributes() []attribute.KeyValue {
	id := hooks.IdentitySource()
	attrs := []attribute.KeyValue{semconv.ServiceVersion(id.Version)}
	if id.Name != "" {
		attrs = append(attrs, semconv.ServiceName(id.Name))
	}
	return attrs
}

func initMeterProvider() error {
	meterProviderOnce.Do(func() {
		promExporter, err := prometheus.New(prometheus.WithoutScopeInfo())
		if err != nil {
			meterProviderErr = fmt.Errorf("metrics: prometheus exporter: %w", err)
			return
		}

		res, err := resource.Merge(resource.Default(),
			resource.NewWithAttributes(semconv.SchemaURL, serviceAttributes()...))
		if err != nil {
			meterProviderErr = fmt.Errorf("metrics: resource merge: %w", err)
			return
		}

		otel.SetMeterProvider(metric.NewMeterProvider(
			metric.WithReader(promExporter),
			metric.WithResource(res),
		))
	})
	return meterProviderErr
}

// NewServerMetrics espone il server ops del processo: /metrics (Prometheus), /health e — solo se
// `metrics.pprof: true` — /debug/pprof/*.
//
// Ritorna error invece di panicare: è un invoke fx, quindi l'errore ferma l'avvio dell'app, che è
// ciò che ci si aspetta da un misconfig. Il listener è aperto dentro OnStart e l'errore è
// propagato: prima l'esito di ListenAndServe finiva in un blocco vuoto, quindi una porta occupata
// era silenzio totale e il processo restava "sano" senza servire nulla. Il ciclo di vita è quello
// di ServeOnLifecycle: un server che muore a regime fa uscire il processo.
//
// Il MeterProvider è inizializzato una volta sola per processo (vedi initMeterProvider): il
// registry Prometheus è globale, quindi è l'unica semantica che non rompe /metrics.
func NewServerMetrics(lc fx.Lifecycle, sh fx.Shutdowner) error {

	if err := initMeterProvider(); err != nil {
		return err
	}

	cfg := withDefaults(hooks.Metrics)

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
		IdleTimeout:       metricsIdleTimeout,
	}

	log.Info().Str("addr", addr).Bool("pprof", cfg.Pprof).Msg("metrics server configured")
	httpx.ServeOnLifecycle(lc, sh, server, "metrics")
	return nil
}
