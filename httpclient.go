package core

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

func AddEndpointNameMetrics(str string, ctx context.Context) context.Context {
	labeler := otelhttp.Labeler{}
	labeler.Add(attribute.String("endpoint", str))
	return otelhttp.ContextWithLabeler(ctx, &labeler)
}

func GenerateHttpClientWithInstrumentation(serviceName string) *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport,
			otelhttp.WithMetricAttributesFn(func(r *http.Request) []attribute.KeyValue {
				return []attribute.KeyValue{attribute.String("service", serviceName)}
			})),
	}
}
