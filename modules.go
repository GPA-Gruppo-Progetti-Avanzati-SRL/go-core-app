package core

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/observability"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/properties"
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
	// defer: un panic di register recuperato più in alto (un test, un wiring che lo intercetta)
	// lascerebbe altrimenti lo stato globale dentro la closure, e le registrazioni successive
	// finirebbero private senza che nessuno l'abbia chiesto.
	defer func() { inPrivate = prev }()
	register()
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
	// Dentro un sottosistema chiuso il confine è il suo: un Module scritto lì dentro ne condivide lo
	// scope, quindi le sue registrazioni sono private come quelle del sottosistema. Prima finiva a
	// root come modulo fratello, e ciò che era stato scritto dentro il confine diventava iniettabile
	// dall'app. Non si apre un fx.Module figlio perché fx non sa esprimere "esportato al solo
	// genitore": un Provide non privato di un figlio sale fino a root, uno privato non lo vedrebbe
	// nemmeno il sottosistema che lo contiene.
	if prev != nil && prev.closed {
		prevPrivate := inPrivate
		inPrivate = false
		func() {
			defer func() { inPrivate = prevPrivate }()
			register()
		}()
		return
	}
	// Un Module annidato dentro una closure Private apre un proprio scope: la privatezza vale per
	// il modulo che l'ha dichiarata, non si eredita in quello nuovo (che ha il suo confine).
	prevPrivate := inPrivate
	inPrivate = false
	ms := &moduleScope{closed: closed}
	current = ms
	// Ripristino in una closure differita per la stessa ragione di Private: senza, un panic
	// recuperato lascerebbe current puntato su uno scope che nessuno costruirà più.
	func() {
		defer func() { current, inPrivate = prev, prevPrivate }()
		register()
	}()

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
	// Annidato in un Module aperto è un figlio suo, non un fratello a root: ne vede i Provide
	// privati (fx.Private vale per il modulo e i suoi discendenti) e compare sotto di lui nel grafo.
	if prev != nil {
		prev.invokes = append(prev.invokes, fx.Module(name, opts...))
		return
	}
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
		// Dentro un Module è un invoke dello scope come gli altri: prima finiva a root senza
		// dirlo, e non vedeva i tipi privati del modulo in cui era stato scritto.
		if current != nil {
			current.invokes = append(current.invokes, fx.Populate(top))
			return
		}
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
	return func() { Invoke(observability.NewTracer, acceptedmodes...) }
}

// WithServerMetrics espone /metrics e /health su 0.0.0.0:2112. Da NON abilitare in mode API:
// go-core-api serve già entrambi sulla porta dell'API, e i due server esporrebbero le stesse
// metriche.
//
//	core.Run(core.WithServerMetrics(engine.Scheduler, engine.Worker))
//
// Indirizzo, ReadHeaderTimeout e l'esposizione di /debug/pprof/* si configurano dalla sezione
// `metrics:` dello YAML (vedi MetricsConfig), non da qui: sono valori che variano per ambiente e
// quasi nessuna app li tocca, quindi non diventano un argomento che ogni chiamante deve scrivere.
//
//	metrics:
//	  pprof: true    # default false; in mode API il gate è invece `develop-mode` di go-core-api
func WithServerMetrics(acceptedmodes ...string) RunOption {
	return func() { Invoke(observability.NewServerMetrics, acceptedmodes...) }
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

// Tag riconosciuti sui campi DIPENDENZA di una struct fornita a ProvideStruct. Sono tag GPA: il
// synthor li traduce nei tag che dig si aspetta, così la struct dell'app non deve conoscere il
// vocabolario di dig. Per i campi di configurazione vedi il package properties (PropTag/DefaultTag/ValidateTag).
//
//	`inject:""`             → dipendenza semplice
//	`inject:"primary"`      → name:"primary"
//	`from:"import_hooks"`   → group:"import_hooks"
//	`optional:"true"`       → optional:"true" (invariato)
const (
	InjectTag   = "inject"
	FromTag     = "from"
	OptionalTag = "optional"
)

var (
	inType    = reflect.TypeFor[In]()
	outType   = reflect.TypeFor[Out]()
	errorType = reflect.TypeFor[error]()
)

// ProvideStruct fornisce a fx un costruttore SINTETIZZATO per il tipo struct T, con questo contratto
// sui campi di T:
//
//	`inject:` / `from:` / `optional:`  → dipendenza: dig la vede (tradotta nei suoi tag)
//	`prop:`                            → property: invisibile a dig, riempita da properties.BindProps
//	nessun tag                         → campo di lavorazione: ignorato, resta al valore zero
//
// La struct NON deve embeddare core.In (è un errore al wiring): il marker lo porta il param object
// sintetico, e accettarlo lascerebbe passare struct scritte per la vecchia semantica, con le
// dipendenze non taggate silenziosamente a nil.
//
//	type importRunner struct {
//	    Data   IData   `inject:"primary"`
//	    Hooks  []IHook `from:"import_hooks"`
//	    Folder string  `prop:"folder" validate:"required"`
//	    buf    []byte  // lavorazione
//	}
//
// mk riceve il *T già popolato (dipendenze iniettate + properties mappate) e ritorna il valore da
// registrare: è l'unico punto che conosce staticamente T ed R.
//
//	owner  etichetta usata negli errori (es. `corekafka: consumer "condizione"`)
//	props  properties da bindare sui campi `prop:` (nil = solo default e validazione)
//	group  value group fx in cui registrare il risultato; "" = provide semplice
//
// Una struct che non si riesce a rappresentare è un errore di programmazione, non di configurazione:
// panic subito, al wiring.
func ProvideStruct[T any, R any](mk func(*T) R, owner string, props properties.Properties, group string, modes ...string) {
	if !IsMode(modes...) {
		return
	}
	ctor, err := synthCtor(reflect.TypeFor[T](), reflect.TypeFor[R](), group, owner, props,
		func(ptr any) any { return mk(ptr.(*T)) })
	if err != nil {
		panic(fmt.Sprintf("%s: %v", owner, err))
	}
	Provide(ctor, modes...)
}

// dep è una dipendenza di T riconosciuta dal synthor: idx è l'indice del campo in T, tag è quello
// tradotto per dig, checkable indica che la verifichiamo noi per dare un errore contestualizzato.
type dep struct {
	idx       int
	tag       reflect.StructTag
	checkable bool
}

// synthCtor sintetizza il costruttore fx per il tipo struct t:
//
//	func(<param object con le SOLE dipendenze di t>) (<risultato, eventualmente nel value group>, error)
//
// Il punto è che dig non vede mai t: vede un param object sintetico che contiene solo i campi
// dipendenza. I campi property e quelli di lavorazione gli sono quindi invisibili e NON richiedono
// `optional:"true"` nell'app; le property le riempie properties.BindProps dopo l'iniezione.
//
// Il costruttore è creato con reflect.MakeFunc, quindi fx non ha una location sensata da mostrare
// (dig.LocationForPC non è esposto da fx) e riporterebbe `reflect.makeFuncStub`. Per non perdere il
// contesto sugli errori più frequenti, le dipendenze **nil-abili** (puntatori, interfacce, mappe,
// slice, chan, func) sono dichiarate `optional:"true"` nel param object sintetico e verificate qui:
// così una dipendenza mancante produce un errore che nomina owner, campo e tipo. Le dipendenze non
// nil-abili (valori struct, param object annidati, interi) restano obbligatorie per dig e in quel caso
// il messaggio è quello di fx, col tipo mancante ma senza location utile.
func synthCtor(t, resultType reflect.Type, group, owner string, props properties.Properties, mk func(ptr any) any) (ctor any, err error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("il tipo deve essere una struct, ricevuto %s", t)
	}
	// reflect.StructOf panica su input che non sa rappresentare: trasformiamo il panic in errore per
	// dare un messaggio contestualizzato al wiring.
	defer func() {
		if r := recover(); r != nil {
			ctor, err = nil, fmt.Errorf("impossibile sintetizzare il costruttore per %s: %v", t, r)
		}
	}()

	deps, err := collectDeps(t, owner)
	if err != nil {
		return nil, err
	}

	// Param object sintetico: marker In + i soli campi dipendenza, con i tag tradotti per dig.
	// Anonymous=false anche per i campi embeddati: a dig serve solo il tipo (un param object annidato
	// è riconosciuto dal tipo, non dall'embedding), e StructOf panica sugli embedded con metodi. Il
	// valore lo ricopiamo per indice.
	fields := []reflect.StructField{{Name: "In", Type: inType, Anonymous: true}}
	for _, d := range deps {
		f := t.Field(d.idx)
		fields = append(fields, reflect.StructField{Name: f.Name, Type: f.Type, Tag: d.tag})
	}
	paramType := reflect.StructOf(fields)

	// Risultato: dentro un value group serve un result object con il marker Out; senza group il
	// costruttore ritorna direttamente R.
	outStructType := resultType
	if group != "" {
		outStructType = reflect.StructOf([]reflect.StructField{
			{Name: "Out", Type: outType, Anonymous: true},
			{Name: "Registration", Type: resultType, Tag: reflect.StructTag(`group:"` + group + `"`)},
		})
	}

	fnType := reflect.FuncOf([]reflect.Type{paramType}, []reflect.Type{outStructType, errorType}, false)
	fn := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		zero := reflect.New(outStructType).Elem()
		fail := func(e error) []reflect.Value {
			return []reflect.Value{zero, reflect.ValueOf(&e).Elem()}
		}

		ptr := reflect.New(t) // *T
		for k, d := range deps {
			ptr.Elem().Field(d.idx).Set(args[0].Field(k + 1))
		}

		for _, d := range deps {
			if !d.checkable {
				continue
			}
			if ptr.Elem().Field(d.idx).IsZero() {
				f := t.Field(d.idx)
				return fail(fmt.Errorf("%s (%s): dipendenza mancante nel grafo fx: campo %s di tipo %s (manca un provider?)",
					owner, t, f.Name, f.Type))
			}
		}

		if bindErr := properties.BindProps(ptr.Interface(), props); bindErr != nil {
			return fail(fmt.Errorf("%s: %w", owner, bindErr))
		}

		res := reflect.New(outStructType).Elem()
		v := reflect.ValueOf(mk(ptr.Interface()))
		if group != "" {
			res.Field(1).Set(v)
		} else {
			res.Set(v)
		}
		return []reflect.Value{res, reflect.Zero(errorType)}
	})

	return fn.Interface(), nil
}

// collectDeps applica il contratto sui campi di t e ritorna le sole dipendenze, con il tag già
// tradotto per dig.
func collectDeps(t reflect.Type, owner string) ([]dep, error) {
	// core.In è il marker di dig e qui non ha senso: il param object lo mette il synthor. Se lo
	// accettassimo, una struct scritta per la vecchia semantica (dipendenze non taggate) vedrebbe i
	// suoi campi trattati come stato interno e resterebbe con le dipendenze a nil, senza che nulla
	// fallisca. Meglio un errore che dice cosa fare.
	if embedsIn(t) {
		return nil, fmt.Errorf("la struct non deve embeddare core.In: le dipendenze si dichiarano col tag `inject:\"\"` (`inject:\"nome\"` per una dipendenza named, `from:\"gruppo\"` per un value group). core.In resta valido nei param object dei costruttori scritti a mano passati a core.Provide")
	}

	var deps []dep
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		_, isProp := f.Tag.Lookup(properties.PropTag)
		injectVal, hasInject := f.Tag.Lookup(InjectTag)
		fromVal, hasFrom := f.Tag.Lookup(FromTag)
		isOptional := f.Tag.Get(OptionalTag) == "true"

		if isProp {
			if hasInject || hasFrom {
				return nil, fmt.Errorf("campo %s: il tag %q non può essere combinato con %q/%q",
					f.Name, properties.PropTag, InjectTag, FromTag)
			}
			continue // property: la riempie properties.BindProps, dig non deve vederla
		}
		if f.PkgPath != "" {
			if hasInject || hasFrom {
				return nil, fmt.Errorf("campo %s: i tag %q/%q richiedono un campo esportato", f.Name, InjectTag, FromTag)
			}
			continue // non esportato: campo di lavorazione
		}
		if !hasInject && !hasFrom && !isOptional {
			continue // nessun tag: campo di lavorazione, dig non lo vede
		}
		if hasFrom && injectVal != "" {
			return nil, fmt.Errorf("campo %s: %q non ammette un nome insieme a %q (dig non supporta i value group named)",
				f.Name, FromTag, InjectTag)
		}
		if hasFrom && fromVal == "" {
			return nil, fmt.Errorf("campo %s: il tag %q richiede il nome del value group", f.Name, FromTag)
		}

		var parts []string
		if injectVal != "" {
			parts = append(parts, `name:"`+injectVal+`"`)
		}
		if fromVal != "" {
			parts = append(parts, `group:"`+fromVal+`"`)
		}
		if isOptional {
			parts = append(parts, `optional:"true"`)
		}
		tag := reflect.StructTag(strings.Join(parts, " "))

		d := dep{idx: i, tag: tag}
		if checkableDep(f.Type, fromVal, isOptional) {
			// La rendiamo opzionale per dig e la verifichiamo noi, per poter dare un errore che nomina
			// owner/campo/tipo invece del generico makeFuncStub.
			d.tag = reflect.StructTag(strings.TrimSpace(string(tag) + ` optional:"true"`))
			d.checkable = true
		}
		deps = append(deps, d)
	}
	return deps, nil
}

// embedsIn indica se t porta il marker core.In (fx.In), che in una struct data a ProvideStruct è un
// errore: il param object sintetico è quello a portare il marker.
func embedsIn(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == inType {
			return true
		}
	}
	return false
}

// checkableDep indica se la dipendenza può essere resa opzionale per dig e verificata da noi: serve un
// tipo nil-abile (per distinguere "assente" da "zero legittimo"), nessun value group (dig rifiuta i
// group opzionali) e nessun optional messo dall'app (che vuole proprio poterla omettere).
func checkableDep(ft reflect.Type, group string, optional bool) bool {
	if group != "" || optional {
		return false
	}
	switch ft.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return true
	default:
		return false
	}
}

// WaitContext attende wg, ma non oltre ctx: ritorna true se wg si è svuotato, false se ctx è
// scaduto prima. È l'attesa di un OnStop — le goroutine in volo devono poter finire, ma una
// appesa non deve tenere in piedi il processo oltre fx.StopTimeout. Cosa fare delle residue lo
// decide il chiamante (di solito: loggarle e proseguire).
//
// Se ctx scade, la goroutine interna che aspetta wg resta viva finché wg non si svuota: è il
// prezzo di non poter interrompere sync.WaitGroup.Wait, e in un processo che sta terminando non
// ha conseguenze.
func WaitContext(ctx context.Context, wg *sync.WaitGroup) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}
