package core

import (
	"net/http"
	netpprof "net/http/pprof"
)

// ProfilingHandler ritorna gli endpoint /debug/pprof/* come handler autonomo, da montare
// esplicitamente sul mux che li deve servire.
//
// Non è un blank import: `_ "net/http/pprof"` registra su http.DefaultServeMux, che questa
// libreria non serve da nessuna parte — gli handler erano quindi irraggiungibili (404) e
// insieme pronti a diventare pubblici se una dipendenza qualsiasi avesse iniziato a servire
// DefaultServeMux. L'esposizione di pprof dev'essere una decisione, non la conseguenza di un
// import.
//
// Index enumera i profili registrati in runtime/pprof, quindi goroutineleak (GA da Go 1.27) è
// incluso senza doverlo nominare qui.
//
// I pattern sono assoluti perché sia http.ServeMux sia chi.Mount presentano all'handler interno
// la r.URL.Path completa: lo stesso handler funziona montato in entrambi i modi, e pprof.Index
// ricava il nome del profilo dal path.
func ProfilingHandler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/debug/pprof/", netpprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", netpprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", netpprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", netpprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", netpprof.Trace)
	return m
}
