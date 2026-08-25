package core

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"slices"

	"github.com/ipfans/fxlogger"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

var provideslist []any
var Mode = os.Getenv("MODE")
var invokelist []fx.Option
var supply []fx.Option
var populatelist []any
var modulelist []fx.Option

// moduleScope raccoglie le registrazioni fatte dentro una chiamata Module(). current
// punta allo scope attivo (nil = scope root, liste globali). La registrazione avviene
// tutta in init() single-thread, quindi lo stato globale è sicuro.
type moduleScope struct {
	provides []any
	supplies []fx.Option
	invokes  []fx.Option
}

var current *moduleScope

type In = fx.In
type Out = fx.Out

// IsMode reports whether the current Mode is among the given modes.
// With no modes it returns true (i.e. "any mode"), coherently with the *If helpers.
func IsMode(acceptedmodes ...string) bool {
	if len(acceptedmodes) == 0 {
		return true
	}
	return slices.Contains(acceptedmodes, Mode)
}

// Provide registra un costruttore/valore. Se acceptedmodes è vuoto registra
// sempre; altrimenti solo se Mode è tra quelli indicati.
//
//	core.Provide(NewData)              // sempre
//	core.Provide(NewData, "batch")     // solo in mode "batch"
func Provide(provide any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		if current != nil {
			current.provides = append(current.provides, provide)
		} else {
			provideslist = append(provideslist, provide)
		}
	}
}

// Supply registra un valore già istanziato. acceptedmodes opzionale come in Provide.
func Supply(value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		if current != nil {
			current.supplies = append(current.supplies, fx.Supply(value))
		} else {
			supply = append(supply, fx.Supply(value))
		}
	}
}

// Module raggruppa in un fx.Module(name) tutte le registrazioni (Provide/Supply/Invoke,
// e i loro wrapper ProvideAs/ProvideNamed/...) effettuate dentro register. Serve solo per
// il namespacing del grafo/log fx: i provide NON sono privati (nessun fx.Private), quindi
// restano visibili all'intera app e i value group aggregano come prima (un consumer nel
// modulo vede anche i produttori a root/antenati). Il mode-gating resta per-registrazione
// (ogni Provide/Supply gate-a con IsMode prima di finire nello scope). Le chiamate fuori da
// Module continuano a registrare nello scope root, quindi è retrocompatibile.
//
//	core.Module("batch", func() { storemongo.Module(); scheduler.Module(cfg) })
func Module(name string, register func()) {
	prev := current
	ms := &moduleScope{}
	current = ms
	register()
	current = prev

	// Nessuna registrazione (es. tutti i componenti gate-ati via dal mode corrente):
	// niente fx.Module vuoto, per non sporcare grafo/log fx.
	if len(ms.provides) == 0 && len(ms.supplies) == 0 && len(ms.invokes) == 0 {
		return
	}

	opts := make([]fx.Option, 0, len(ms.supplies)+len(ms.invokes)+1)
	opts = append(opts, ms.supplies...)
	if len(ms.provides) > 0 {
		opts = append(opts, fx.Provide(ms.provides...))
	}
	opts = append(opts, ms.invokes...)
	modulelist = append(modulelist, fx.Module(name, opts...))
}

// ProvideAs registra ctor annotandolo per essere fornito come l'interfaccia T,
// eliminando il boilerplate fx.Annotate(ctor, fx.As(new(T))). Il costruttore si
// passa nudo; l'interfaccia è il type parameter. acceptedmodes opzionale.
//
//	core.ProvideAs[IData](NewData)
//	core.ProvideAs[IData](NewData, engine.Batch, engine.Worker)
func ProvideAs[T any](ctor any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		Provide(fx.Annotate(ctor, fx.As(new(T))))
	}
}

// ProvideWith registra un costruttore insieme al valore (tipicamente il config)
// che consuma, in un'unica chiamata. acceptedmodes opzionale.
func ProvideWith(provide any, value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		Provide(provide)
		Supply(value)
	}
}

// ProvideAsWith è ProvideWith con il costruttore registrato come l'interfaccia T.
//
//	core.ProvideAsWith[IClient](NewService, &cfg.C, engine.Batch, engine.Worker, engine.Api)
func ProvideAsWith[T any](ctor any, value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		ProvideAs[T](ctor)
		Supply(value)
	}
}

// ProvideNamed registra ctor annotandolo con un nome (fx.ResultTags(`name:"..."`)),
// eliminando il boilerplate fx.Annotate(ctor, fx.ResultTags(`name:"..."`)). Il
// costruttore si passa nudo come primo parametro; acceptedmodes opzionale.
//
//	core.ProvideNamed(locker.NewCatalogoLocker, "catalogo")
//	core.ProvideNamed(locker.NewJobLocker, "job", engine.Batch)
func ProvideNamed(ctor any, name string, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		Provide(fx.Annotate(ctor, fx.ResultTags(`name:"`+name+`"`)))
	}
}

// ProvideNamedWith è ProvideNamed con il valore (tipicamente il config) che il
// costruttore consuma, fornito nella stessa chiamata (posizione coerente con ProvideWith).
//
//	core.ProvideNamedWith(mngr.NewClient, &cfg.MngrConfig, "mngr")
func ProvideNamedWith(ctor any, name string, value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		ProvideNamed(ctor, name)
		Supply(value)
	}
}

// ProvideAsNamed registra ctor annotandolo sia come interfaccia T (fx.As) sia con
// un nome (fx.ResultTags), combinando ProvideAs e ProvideNamed.
//
//	core.ProvideAsNamed[IClient](svc.New, "primary")
func ProvideAsNamed[T any](ctor any, name string, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		Provide(fx.Annotate(ctor, fx.As(new(T)), fx.ResultTags(`name:"`+name+`"`)))
	}
}

// ProvideAsNamedWith è ProvideAsNamed con il valore consumato dal costruttore.
//
//	core.ProvideAsNamedWith[IClient](svc.New, &cfg.C, "primary")
func ProvideAsNamedWith[T any](ctor any, name string, value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		ProvideAsNamed[T](ctor, name)
		Supply(value)
	}
}

// Invoke registra una funzione eseguita all'avvio (side-effect). acceptedmodes opzionale.
func Invoke(invoke any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		if current != nil {
			current.invokes = append(current.invokes, fx.Invoke(invoke))
		} else {
			invokelist = append(invokelist, fx.Invoke(invoke))
		}
	}
}

// Populate registra un target per fx.Populate. acceptedmodes opzionale.
func Populate(top any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		populatelist = append(populatelist, top)
	}
}

func invokes() fx.Option {
	return fx.Options(invokelist...)
}

func populates() fx.Option {
	return fx.Populate(populatelist...)
}

func provides() fx.Option {
	// Buffer locale: usare la globale supply come accumulatore renderebbe provides() non
	// idempotente (un secondo configureApp fornirebbe due volte gli stessi costruttori).
	opts := make([]fx.Option, 0, len(supply)+1)
	opts = append(opts, supply...)
	if len(provideslist) > 0 {
		opts = append(opts, fx.Provide(provideslist...))
	}
	return fx.Options(opts...)
}

// RunOption è un componente standard che l'app abilita al momento di Run. Registra come le altre
// (Provide/Invoke), quindi vale il solito gating per mode.
type RunOption func()

// WithTracing abilita il TracerProvider OpenTelemetry.
//
//	core.Run(core.WithTracing())
func WithTracing(acceptedmodes ...string) RunOption {
	return func() { Invoke(NewTracer, acceptedmodes...) }
}

// WithServerMetrics espone /metrics e /health su :2112. Da NON abilitare in mode API: go-core-api
// serve già entrambi sulla porta dell'API, e i due server esporrebbero le stesse metriche.
//
//	core.Run(core.WithServerMetrics(engine.Scheduler, engine.Worker))
func WithServerMetrics(acceptedmodes ...string) RunOption {
	return func() { Invoke(NewServerMetrics, acceptedmodes...) }
}

func Run(opts ...RunOption) {
	for _, o := range opts {
		o()
	}
	app := configureApp()
	app.Run()
}

func Start(ctx context.Context, opts ...RunOption) (*fx.App, error) {
	for _, o := range opts {
		o()
	}
	app := configureApp()
	err := app.Start(ctx)
	return app, err
}

func configureApp() *fx.App {

	fmt.Printf("%s\nVersion: %s\nSha: %s\nBuildDate: %s\nRuntime: %s\nOS: %s\nArch: %s\nNumCPU: %d\nGOMAXPROCS: %d\nGOMEMLIMIT=%s\n", string(Logo), BuildVersion, SHA, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), FormatBytes(debug.SetMemoryLimit(-1)))
	if Mode != "" {
		fmt.Printf("Mode: %s\n", Mode)
	}
	return fx.New(
		fx.WithLogger(fxlogger.WithZerolog(log.Logger)),
		provides(),
		populates(),
		invokes(),
		fx.Options(modulelist...),
	)
}
