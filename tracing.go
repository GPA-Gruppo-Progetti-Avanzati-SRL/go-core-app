package core

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/propagators/autoprop"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

func init() {
	ctx := context.Background()
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
	)

	if err != nil {
		panic(err)
	}

	traceExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		panic(err)
	}

	traceProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter,
			// Default is 5s. Set to 1s for demonstrative purposes.
			trace.WithBatchTimeout(time.Second)),
		trace.WithResource(res),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(autoprop.NewTextMapPropagator())

}
