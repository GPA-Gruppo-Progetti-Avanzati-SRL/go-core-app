package core

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/fx/fxtest"
)

// startMetrics avvia il server ops con la sezione `metrics:` data e ritorna il suo base URL.
// metricsConfig è la var di package che ReadConfig popola: qui la si imposta a mano, che è
// esattamente ciò che fa la lettura della config.
func startMetrics(t *testing.T, cfg MetricsConfig) string {
	t.Helper()

	prev := metricsConfig
	metricsConfig = cfg
	t.Cleanup(func() { metricsConfig = prev })

	lc := fxtest.NewLifecycle(t)
	if err := NewServerMetrics(lc); err != nil {
		t.Fatalf("NewServerMetrics: %v", err)
	}
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	eff := metricsConfig.withDefaults()
	return fmt.Sprintf("http://127.0.0.1:%d", eff.Port)
}

func status(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestServerMetrics_DefaultSenzaSezioneMetrics: una config che non nomina `metrics:` deve
// comportarsi come prima della modifica — /metrics e /health serviti, pprof NON esposto.
func TestServerMetrics_DefaultSenzaSezioneMetrics(t *testing.T) {
	base := startMetrics(t, MetricsConfig{Port: freePort(t)})

	if got := status(t, base+"/metrics"); got != http.StatusOK {
		t.Errorf("/metrics: status %d, atteso 200", got)
	}
	if got := status(t, base+"/health"); got != http.StatusOK {
		t.Errorf("/health: status %d, atteso 200", got)
	}
	if got := status(t, base+"/debug/pprof/"); got != http.StatusNotFound {
		t.Errorf("/debug/pprof/ con pprof spento: status %d, atteso 404", got)
	}
}

// TestServerMetrics_PprofAbilitato: con `metrics.pprof: true` il claim della documentazione è vero.
func TestServerMetrics_PprofAbilitato(t *testing.T) {
	base := startMetrics(t, MetricsConfig{Port: freePort(t), Pprof: true})

	if got := status(t, base+"/debug/pprof/goroutineleak?debug=1"); got != http.StatusOK {
		t.Errorf("/debug/pprof/goroutineleak: status %d, atteso 200", got)
	}
	// /metrics deve restare servito anche quando pprof è acceso, e anche se questo è il secondo
	// NewServerMetrics del processo: il MeterProvider è inizializzato una volta sola, altrimenti
	// il registry Prometheus avrebbe le metric family duplicate e lo scrape risponderebbe 500.
	if got := status(t, base+"/metrics"); got != http.StatusOK {
		t.Errorf("/metrics deve restare servito: status %d", got)
	}
	if got := status(t, base+"/health"); got != http.StatusOK {
		t.Errorf("/health deve restare servito: status %d", got)
	}
}

// TestServerMetrics_ScrapeReggeChiamateRipetute: il MeterProvider è stato globale di processo, e
// registrarlo due volte duplicava le metric family → 500 allo scrape. È il caso che `go test
// -count=2` della CI riproduce, quindi va asserito qui e non lasciato scoprire alla pipeline.
func TestServerMetrics_ScrapeReggeChiamateRipetute(t *testing.T) {
	for i := range 3 {
		base := startMetrics(t, MetricsConfig{Port: freePort(t)})
		if got := status(t, base+"/metrics"); got != http.StatusOK {
			t.Fatalf("/metrics alla chiamata %d: status %d, atteso 200", i+1, got)
		}
	}
}

// TestMetricsConfig_Defaults: i default riempiono i soli campi non valorizzati, e coincidono con
// quelli dichiarati a viper in ReadConfig. Pprof non è fra questi: false è una scelta, non un
// "non valorizzato".
func TestMetricsConfig_Defaults(t *testing.T) {
	got := MetricsConfig{}.withDefaults()

	if got.Host != DefaultMetricsHost {
		t.Errorf("Host = %q, atteso %q (tutte le interfacce: Prometheus scrapa l'IP del pod)", got.Host, DefaultMetricsHost)
	}
	if got.Port != DefaultMetricsPort {
		t.Errorf("Port = %d, atteso %d", got.Port, DefaultMetricsPort)
	}
	if got.ReadHeaderTimeout != DefaultMetricsReadHeaderTimeout {
		t.Errorf("ReadHeaderTimeout = %v, atteso %v", got.ReadHeaderTimeout, DefaultMetricsReadHeaderTimeout)
	}
	if got.Pprof {
		t.Error("Pprof deve restare false per default: l'esposizione si chiede, non si eredita")
	}

	explicit := MetricsConfig{Host: "127.0.0.1", Port: 9999, ReadHeaderTimeout: time.Second}.withDefaults()
	if explicit.Host != "127.0.0.1" || explicit.Port != 9999 || explicit.ReadHeaderTimeout != time.Second {
		t.Errorf("i valori espliciti non devono essere sovrascritti: %+v", explicit)
	}
}

// TestServerMetrics_PortaOccupataFallisceOnStart: prima l'errore di ListenAndServe finiva in un
// blocco vuoto — porta occupata = silenzio, e il processo passava l'health check senza servire
// nulla (§4.7). Ora deve fallire l'avvio.
func TestServerMetrics_PortaOccupataFallisceOnStart(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port

	prev := metricsConfig
	metricsConfig = MetricsConfig{Host: "127.0.0.1", Port: port}
	defer func() { metricsConfig = prev }()

	lc := fxtest.NewLifecycle(t)
	if err := NewServerMetrics(lc); err != nil {
		t.Fatalf("NewServerMetrics: %v", err)
	}

	if err := lc.Start(context.Background()); err == nil {
		_ = lc.Stop(context.Background())
		t.Fatal("OnStart doveva fallire su porta occupata, invece è andato a buon fine")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
