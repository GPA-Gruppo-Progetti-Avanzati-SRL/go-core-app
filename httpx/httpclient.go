package httpx

import (
	"context"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

func AddEndpointNameMetrics(str string, ctx context.Context) context.Context {
	labeler := otelhttp.Labeler{}
	labeler.Add(attribute.String("endpoint", str))
	return otelhttp.ContextWithLabeler(ctx, &labeler)
}

// DefaultHttpClientTimeout è il timeout dei client di GenerateHttpClientWithInstrumentation. Un
// http.Client a zero non ha timeout: un upstream che accetta la connessione e non risponde teneva
// ferma la goroutine chiamante per sempre, e con lei la richiesta che la aspettava.
const DefaultHttpClientTimeout = 30 * time.Second

// GenerateHttpClientWithInstrumentation ritorna un client strumentato OTel con l'attributo
// `service` e un timeout complessivo per richiesta. Il timeout è DefaultHttpClientTimeout, oppure
// il primo valore > 0 passato: il parametro è variadico solo per non rompere le chiamate
// esistenti. Un deadline più stretto per la singola chiamata si mette sul context della richiesta.
func GenerateHttpClientWithInstrumentation(serviceName string, timeout ...time.Duration) *http.Client {
	t := DefaultHttpClientTimeout
	if len(timeout) > 0 && timeout[0] > 0 {
		t = timeout[0]
	}
	return &http.Client{
		Timeout: t,
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
	// WithContext e non Clone: un RoundTripper può sostituire il context su una copia superficiale,
	// e Clone copiava in profondità gli header a ogni richiesta senza che nessuno li modifichi.
	return t.base.RoundTrip(req.WithContext(otelhttp.ContextWithLabeler(req.Context(), labeler)))
}
