package observability

import (
	"context"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.uber.org/fx"
)

type Tracer struct {
	TracerProvider *trace.TracerProvider
}

// NewTracer installa il TracerProvider OTLP, salvo che il tracing sia spento dall'ambiente:
//
//   - OTEL_SDK_DISABLED=true (la variabile della spec OTel, che l'SDK Go non legge da sé);
//   - nessuna destinazione: né OTEL_TRACES_EXPORTER né OTEL_EXPORTER_OTLP_[TRACES_]ENDPOINT.
//     Senza, autoexport ricadrebbe su localhost:4318 e in locale ogni export fallirebbe;
//   - OTEL_TRACES_EXPORTER=none: l'exporter scarterebbe tutto, ma il provider continuerebbe a
//     campionare e registrare ogni span.
//
// Da spento resta il provider noop globale di otel, mentre il propagator è impostato comunque: un
// traceparent in ingresso viene ancora propagato alle chiamate in uscita, quindi un servizio senza
// tracing non spezza la traccia degli altri.
func NewTracer(lc fx.Lifecycle) *Tracer {

	tracer := new(Tracer)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

			if reason := tracingDisabledReason(); reason != "" {
				log.Info().Str("reason", reason).Msg("tracing disabilitato")
				return nil
			}

			// Nome e versione dell'app come base, l'ambiente sopra: senza service.name ogni span
			// arrivava come `unknown_service` salvo OTEL_SERVICE_NAME, che resta comunque
			// vincente perché WithFromEnv è applicato per ultimo.
			res, err := resource.New(ctx,
				resource.WithSchemaURL(semconv.SchemaURL),
				resource.WithAttributes(serviceAttributes()...),
				resource.WithFromEnv(),
			)

			if err != nil {
				return err
			}

			traceExporter, err := autoexport.NewSpanExporter(ctx)
			if err != nil {
				return err
			}
			if autoexport.IsNoneSpanExporter(traceExporter) {
				log.Info().Str("reason", "OTEL_TRACES_EXPORTER=none").Msg("tracing disabilitato")
				return nil
			}

			// Gli errori dell'SDK (export falliti compresi) finivano sul log stdlib, fuori dallo
			// stream e dal formato dell'app.
			otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
				log.Warn().Str("component", "otel").Err(err).Msg("otel")
			}))

			traceProvider := trace.NewTracerProvider(
				// Batch timeout al default dell'SDK (5s, o OTEL_BSP_SCHEDULE_DELAY): l'1s
				// "dimostrativo" quintuplicava gli export senza che nessuno l'avesse chiesto.
				trace.WithBatcher(traceExporter),
				trace.WithResource(res),
			)

			otel.SetTracerProvider(traceProvider)

			tracer.TracerProvider = traceProvider

			return nil

		},
		OnStop: func(ctx context.Context) error {
			if tracer.TracerProvider != nil {
				return tracer.TracerProvider.Shutdown(ctx)
			}
			return nil
		}})
	return tracer
}

// tracingDisabledReason ritorna perché l'ambiente spegne il tracing, o "" se va acceso.
func tracingDisabledReason() string {
	switch v := os.Getenv("OTEL_SDK_DISABLED"); {
	case strings.EqualFold(v, "true"):
		return "OTEL_SDK_DISABLED=true"
	case v != "" && !strings.EqualFold(v, "false"):
		// La spec vuole che un valore non riconosciuto valga false: lo si rispetta, ma un typo
		// che lascia acceso ciò che si voleva spegnere va detto.
		log.Warn().Str("OTEL_SDK_DISABLED", v).Msg("valore non riconosciuto (attesi true/false): tracing attivo")
	}
	for _, k := range []string{"OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		if os.Getenv(k) != "" {
			return ""
		}
	}
	return "nessuna destinazione (OTEL_EXPORTER_OTLP_ENDPOINT o OTEL_TRACES_EXPORTER non impostate)"
}
