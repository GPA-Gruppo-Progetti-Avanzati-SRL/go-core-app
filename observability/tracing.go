package observability

import (
	"context"

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

func NewTracer(lc fx.Lifecycle) *Tracer {

	tracer := new(Tracer)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
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

			traceProvider := trace.NewTracerProvider(
				// Batch timeout al default dell'SDK (5s, o OTEL_BSP_SCHEDULE_DELAY): l'1s
				// "dimostrativo" quintuplicava gli export senza che nessuno l'avesse chiesto.
				trace.WithBatcher(traceExporter),
				trace.WithResource(res),
			)

			otel.SetTracerProvider(traceProvider)
			otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

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
