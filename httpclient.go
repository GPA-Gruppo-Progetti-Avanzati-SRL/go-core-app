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
		Transport: serviceLabeler{
			base: otelhttp.NewTransport(http.DefaultTransport),
			attr: attribute.String("service", serviceName),
		},
	}
}

// serviceLabeler aggiunge l'attributo `service` alle metriche di OGNI richiesta fatta con questo
// client. Sostituisce otelhttp.WithMetricAttributesFn, deprecata in favore del Labeler — che è lo
// stesso meccanismo già usato da AddEndpointNameMetrics per l'attributo `endpoint`.
//
// Il labeler non viene preso da quello del chiamante ma ricostruito: un RoundTripper non deve
// modificare ciò che riceve, e mutare il labeler del context lo lascerebbe con un `service` in più
// a ogni richiesta se lo stesso context servisse più chiamate. Gli attributi già presenti (es.
// l'`endpoint`) vengono ricopiati, quindi non se ne perde nessuno.
type serviceLabeler struct {
	base http.RoundTripper
	attr attribute.KeyValue
}

func (t serviceLabeler) RoundTrip(req *http.Request) (*http.Response, error) {
	labeler := &otelhttp.Labeler{}
	if parent, found := otelhttp.LabelerFromContext(req.Context()); found {
		labeler.Add(parent.Get()...)
	}
	labeler.Add(t.attr)
	return t.base.RoundTrip(req.Clone(otelhttp.ContextWithLabeler(req.Context(), labeler)))
}
