package core

import (
	"context"
	"os"
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
	// privates sono i Provide che il modulo tiene per sé anche quando i suoi Provide sono
	// esportati (vedi Private). In un ModuleClosed non serve — lì è privato tutto.
	privates []any
	supplies []any
	invokes  []fx.Option
	closed   bool
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
			if inPrivate {
				current.privates = append(current.privates, provide)
			} else {
				current.provides = append(current.provides, provide)
			}
		} else {
			provideslist = append(provideslist, provide)
		}
	}
}

// inPrivate dice se la registrazione in corso avviene dentro una closure Private. È stato
// globale come current, e per la stessa ragione: il wiring è single-thread.
var inPrivate bool

// Private rende PRIVATI al modulo corrente i Provide registrati dentro register, anche quando il
// modulo esporta i propri (Module). Serve quando il gruppo di registrazioni che compone un servizio
// contiene sia il servizio — che l'app deve poter iniettare — sia i suoi ingranaggi, che non le
// servono e che due moduli fratelli potrebbero fornire entrambi:
//
//	core.Module("kafka-producer", func() {
//	    core.Supply(cfg.Server)              // privato: Supply in un Module lo è già
//	    core.Private(driver)                 // il driver.Factory resta dentro
//	    core.Provide(newProducer)            // il servizio esce
//	})
//
// Senza, l'unica granularità sarebbe il modulo intero: o tutto esportato (Module) o niente
// (ModuleClosed), e un ingranaggio esportato da due moduli fratelli è un duplicate provide.
//
// Panica fuori da un Module/ModuleClosed: a root non esiste uno scope da cui nascondersi, e non
// fare nulla in silenzio sarebbe la risposta peggiore.
func Private(register func()) {
	if current == nil {
		panic("core.Private: chiamabile solo dentro core.Module o core.ModuleClosed (a root non c'è uno scope in cui essere privati)")
	}
	prev := inPrivate
	inPrivate = true
	register()
	inPrivate = prev
}

// Supply registra un valore già istanziato. acceptedmodes opzionale come in Provide.
//
// Dentro un Module/ModuleClosed il valore è supplito con fx.Private, quindi è visibile solo ai
// costruttori di quel modulo: la config di un servizio è un dettaglio del servizio, non una
// dipendenza condivisa. A root resta pubblica — è così che la config applicativa supplita da
// Boot rimane l'unica iniettabile da tutto il grafo. Il valore è accumulato nudo, perché è
// buildModule a decidere la visibilità al momento di costruire l'fx.Module.
func Supply(value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		if current != nil {
			current.supplies = append(current.supplies, value)
		} else {
			supply = append(supply, fx.Supply(value))
		}
	}
}

// Module raggruppa in un fx.Module(name) tutte le registrazioni (Provide/Supply/Invoke, e i
// loro wrapper ProvideAs/ProvideNamed/...) effettuate dentro register. È la primitiva dei
// DRIVER — le librerie che esistono per dare un handle all'app (mongo, sql, redis) — e delle
// registrazioni dell'app stessa:
//
//   - i Supply sono PRIVATI al modulo (fx.Private): la config di un servizio non è iniettabile
//     da fuori;
//   - i Provide restano ESPORTATI: *Service, *bun.DB, client e interfacce sono consumabili
//     dall'intera app, e i value group aggregano come prima (un consumer nel modulo vede anche
//     i produttori a root/antenati).
//
// Il mode-gating resta per-registrazione (ogni Provide/Supply gate-a con IsMode prima di finire
// nello scope). Le chiamate fuori da Module registrano nello scope root, quindi è retrocompatibile.
//
//	core.Module("mongo", func() { core.Supply(cfg); core.Provide(newService) })
func Module(name string, register func()) {
	buildModule(name, register, false)
}

// ModuleClosed è Module per i SOTTOSISTEMI CHIUSI — le librerie che consumano i seam dell'app
// (api: rotte+business; kafka: Handler/Transformer; batch: ITaskRunner) e non le devono esporre
// nulla in cambio: qui sono privati sia i Supply sia i Provide, quindi *Router, *Scheduler,
// *Consumers, dispatcher, feed e i loro config non sono iniettabili dal grafo dell'app.
//
// Cosa continua ad attraversare il confine:
//
//   - dall'esterno verso l'interno: tutto ciò che è esportato a root (il business dell'app, il
//     *coremongo.Service, gli Handler/runner forniti dall'app) è visibile ai costruttori del
//     modulo, che ne è discendente — compresi i membri di value group forniti a root;
//
//   - dall'interno verso l'esterno: nulla. Un seam pubblico si esprime registrandolo FUORI dallo
//     scope (è ciò che fa batch con store.IWorkItemStore/store.IData, wirati a root).
//
//     core.ModuleClosed("api", func() { core.Supply(cfg); core.Provide(newRouter) })
func ModuleClosed(name string, register func()) {
	buildModule(name, register, true)
}

func buildModule(name string, register func(), closed bool) {
	prev := current
	// Un Module annidato dentro una closure Private apre un proprio scope: la privatezza vale per
	// il modulo che l'ha dichiarata, non si eredita in quello nuovo (che ha il suo confine).
	prevPrivate := inPrivate
	inPrivate = false
	ms := &moduleScope{closed: closed}
	current = ms
	register()
	current = prev
	inPrivate = prevPrivate

	// Nessuna registrazione (es. tutti i componenti gate-ati via dal mode corrente):
	// niente fx.Module vuoto, per non sporcare grafo/log fx.
	if len(ms.provides) == 0 && len(ms.privates) == 0 && len(ms.supplies) == 0 && len(ms.invokes) == 0 {
		return
	}

	opts := make([]fx.Option, 0, len(ms.supplies)+len(ms.invokes)+2)
	for _, v := range ms.supplies {
		opts = append(opts, fx.Supply(v, fx.Private))
	}
	if len(ms.provides) > 0 {
		if ms.closed {
			// fx.Private tra i target marca privati tutti i costruttori passati nella stessa
			// chiamata (fx/provide.go: provideOption.apply).
			opts = append(opts, fx.Provide(append(ms.provides, fx.Private)...))
		} else {
			opts = append(opts, fx.Provide(ms.provides...))
		}
	}
	// I Provide dentro Private vanno in una chiamata SEPARATA, perché fx.Private marca l'intera
	// chiamata: mescolarli con gli altri renderebbe privato anche ciò che il modulo esporta.
	if len(ms.privates) > 0 {
		opts = append(opts, fx.Provide(append(ms.privates, fx.Private)...))
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

	app := fx.New(
		fx.WithLogger(fxlogger.WithZerolog(log.Logger)),
		provides(),
		populates(),
		invokes(),
		fx.Options(modulelist...),
	)

	// Le liste sono un accumulatore PER LA PROSSIMA app, non lo stato di quella appena costruita:
	// fx.New ha già copiato ciò che le serve.
	//
	// Un'applicazione chiama Run *oppure* Start — sono duali — quindi il punto NON è l'idempotenza
	// di una seconda chiamata dentro la stessa app. Il punto è che senza lo svuotamento **non si può
	// costruire più di una fx.App nello stesso processo**, e i test sono esattamente quel caso: con
	// `go test -count=2` lo stesso test gira due volte, il suo Supply resta nella lista e dig
	// fallisce con "already provided" (è il rosso storico di go-core-batch/simplejob).
	resetRegistry()
	return app
}

// resetRegistry svuota l'accumulatore delle registrazioni. È l'unico punto che elenca le liste:
// una seconda copia dell'elenco sarebbe una copia da tenere allineata, e dimenticarne una lì
// significa uno stato che sopravvive senza che nulla lo segnali.
func resetRegistry() {
	provideslist = nil
	invokelist = nil
	supply = nil
	populatelist = nil
	modulelist = nil
	current = nil
	inPrivate = false
}
