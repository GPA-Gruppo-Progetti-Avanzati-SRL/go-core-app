package core

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/fx"
)

func init() {
	Provides(NewOtelTracer)
	Invoke(NewOtelTracer)
}

type Tracer struct {
	TracerProvider *trace.TracerProvider
}

func NewOtelTracer(lc fx.Lifecycle) *Tracer {

	tracer := new(Tracer)
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			res, err := resource.New(ctx,
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
				trace.WithBatcher(traceExporter,
					// Default is 5s. Set to 1s for demonstrative purposes.
					trace.WithBatchTimeout(time.Second)),
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
