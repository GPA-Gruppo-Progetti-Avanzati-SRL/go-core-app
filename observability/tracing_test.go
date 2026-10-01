package observability

import (
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/fx/fxtest"
)

// startTracer avvia NewTracer con l'ambiente dato (le variabili non citate sono azzerate) e
// ripristina i globali di otel a fine test. Non parallelo: env e provider sono di processo.
func startTracer(t *testing.T, env map[string]string) *Tracer {
	t.Helper()
	for _, k := range []string{"OTEL_SDK_DISABLED", "OTEL_TRACES_EXPORTER", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		t.Setenv(k, env[k])
	}
	// Non il provider corrente: all'inizio è il delegato globale, e reinstallarlo è rifiutato.
	t.Cleanup(func() { otel.SetTracerProvider(noop.NewTracerProvider()) })

	lc := fxtest.NewLifecycle(t)
	tr := NewTracer(lc)
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)
	return tr
}

func TestNewTracer_Ambiente(t *testing.T) {
	cases := []struct {
		name   string
		env    map[string]string
		attivo bool
	}{
		{"nessuna destinazione", nil, false},
		{"OTEL_SDK_DISABLED vince sull'endpoint", map[string]string{"OTEL_SDK_DISABLED": "TRUE", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318"}, false},
		{"exporter none", map[string]string{"OTEL_TRACES_EXPORTER": "none"}, false},
		{"exporter console senza endpoint", map[string]string{"OTEL_TRACES_EXPORTER": "console"}, true},
		{"OTEL_SDK_DISABLED non riconosciuto", map[string]string{"OTEL_SDK_DISABLED": "yes", "OTEL_TRACES_EXPORTER": "console"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := startTracer(t, c.env)
			_, sdk := otel.GetTracerProvider().(*sdktrace.TracerProvider)
			if got := tr.TracerProvider != nil; got != c.attivo || sdk != c.attivo {
				t.Fatalf("attivo = %v (provider globale sdk = %v), atteso %v", got, sdk, c.attivo)
			}
		})
	}
}
