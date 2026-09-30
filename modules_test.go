package core

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/properties"
	"go.uber.org/fx"
)

// TestModule verifica il namespacing via core.Module (Depth 1, no fx.Private):
//   - le registrazioni fatte dentro Module NON finiscono nelle liste root, ma in un fx.Module;
//   - i provide del modulo restano visibili all'app (hoisted): un consumer DENTRO il modulo
//     vede sia i produttori di gruppo forniti a root (scenario runner app→batch) sia quelli
//     del modulo stesso.
func TestModule(t *testing.T) {
	resetLists()

	// Produttore di gruppo a ROOT (simula runner.Provide fatto dall'app).
	Provide(fx.Annotate(func() string { return "root-runner" }, fx.ResultTags(`group:"runners"`)))

	var seen []string
	Module("batch", func() {
		// Produttore di gruppo DENTRO il modulo (simula s3feed).
		Provide(fx.Annotate(func() string { return "batch-runner" }, fx.ResultTags(`group:"runners"`)))
		// Consumer del gruppo DENTRO il modulo (simula localdispatcher/grpchandler).
		Invoke(fx.Annotate(func(rs []string) { seen = rs }, fx.ParamTags(`group:"runners"`)))
	})

	// Routing: il provide del modulo non è nella lista root; il modulo è in modulelist.
	if len(provideslist) != 1 {
		t.Fatalf("root provideslist = %d, want 1 (solo il root-runner)", len(provideslist))
	}
	if len(modulelist) != 1 {
		t.Fatalf("modulelist = %d, want 1 (fx.Module batch)", len(modulelist))
	}

	app := fx.New(provides(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New error: %v", err)
	}
	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}
	defer func() { _ = app.Stop(ctx) }()

	if len(seen) != 2 {
		t.Fatalf("consumer nel modulo batch ha visto %d membri, want 2 (root-runner + batch-runner): %v", len(seen), seen)
	}
}

// TestModuleEmpty verifica che un core.Module in cui non viene registrato nulla (es. tutti
// i componenti gate-ati via dal mode corrente) NON produca un fx.Module vuoto.
func TestModuleEmpty(t *testing.T) {
	resetLists()
	Module("api", func() {
		// Simula api.Module(engine.Api) girando in un mode diverso da Api:
		Provide(func() string { return "unreachable" }, "MODE_CHE_NON_MATCHA")
	})
	if len(modulelist) != 0 {
		t.Fatalf("modulelist = %d, want 0 (nessun fx.Module vuoto)", len(modulelist))
	}
}

// --- Visibilità: driver aperti (Module) vs sottosistemi chiusi (ModuleClosed) ---

type svcConfig struct{ Url string }

// TestModuleSupplyPrivate: la config supplita dentro un Module è visibile ai soli costruttori del
// modulo. È l'invariante "la config di un servizio non è iniettabile dall'app".
func TestModuleSupplyPrivate(t *testing.T) {
	t.Run("consumer a root non la vede", func(t *testing.T) {
		resetLists()
		Module("mongo", func() { Supply(&svcConfig{Url: "mongodb://x"}) })
		Invoke(func(*svcConfig) {})

		if err := fx.New(provides(), invokes(), fx.Options(modulelist...)).Err(); err == nil {
			t.Fatal("fx.New senza errore: la config supplita nel modulo è ancora iniettabile a root")
		}
	})

	t.Run("consumer nel modulo la vede", func(t *testing.T) {
		resetLists()
		var got string
		Module("mongo", func() {
			Supply(&svcConfig{Url: "mongodb://x"})
			Invoke(func(c *svcConfig) { got = c.Url })
		})

		app := fx.New(provides(), fx.Options(modulelist...))
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		startStop(t, app)
		if got != "mongodb://x" {
			t.Fatalf("config nel modulo = %q, want mongodb://x", got)
		}
	})
}

// TestRootSupplyStaysPublic: il Supply a root resta pubblico — è il caso della config applicativa
// supplita da Boot, l'unica che tutto il grafo può iniettare.
func TestRootSupplyStaysPublic(t *testing.T) {
	resetLists()
	var got string
	Supply(&svcConfig{Url: "app-config"})
	Module("api", func() { Invoke(func(c *svcConfig) { got = c.Url }) })

	app := fx.New(provides(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New error: %v", err)
	}
	startStop(t, app)
	if got != "app-config" {
		t.Fatalf("config a root vista nel modulo = %q, want app-config", got)
	}
}

// TestModuleProvidesStayExported: in un Module (driver) i provide restano esportati — è il caso
// *coremongo.Service consumato dal data layer dell'app a root. Regressione da non introdurre.
func TestModuleProvidesStayExported(t *testing.T) {
	resetLists()
	var got string
	Module("mongo", func() { Provide(func() *svcConfig { return &svcConfig{Url: "service"} }) })
	Invoke(func(s *svcConfig) { got = s.Url })

	app := fx.New(provides(), invokes(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New error: %v", err)
	}
	startStop(t, app)
	if got != "service" {
		t.Fatalf("provide del modulo visto a root = %q, want service", got)
	}
}

// TestModuleClosedProvidesPrivate: in un ModuleClosed (api/kafka/batch) anche i provide sono privati
// — *Router, *Scheduler, *Consumers non sono iniettabili dal grafo dell'app.
func TestModuleClosedProvidesPrivate(t *testing.T) {
	t.Run("consumer a root non lo vede", func(t *testing.T) {
		resetLists()
		ModuleClosed("api", func() { Provide(func() *svcConfig { return &svcConfig{Url: "router"} }) })
		Invoke(func(*svcConfig) {})

		if err := fx.New(provides(), invokes(), fx.Options(modulelist...)).Err(); err == nil {
			t.Fatal("fx.New senza errore: il provide del sottosistema chiuso è ancora iniettabile a root")
		}
	})

	t.Run("consumer nel modulo lo vede", func(t *testing.T) {
		resetLists()
		var got string
		ModuleClosed("api", func() {
			Provide(func() *svcConfig { return &svcConfig{Url: "router"} })
			Invoke(func(s *svcConfig) { got = s.Url })
		})

		app := fx.New(provides(), fx.Options(modulelist...))
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		startStop(t, app)
		if got != "router" {
			t.Fatalf("provide nel modulo chiuso = %q, want router", got)
		}
	})
}

// TestModuleClosedSeesRoot: dentro un sottosistema chiuso i seam dell'app restano visibili — il
// business/data layer per tipo e i membri di value group forniti a root (handler kafka, runner batch).
func TestModuleClosedSeesRoot(t *testing.T) {
	resetLists()
	Provide(func() *svcConfig { return &svcConfig{Url: "business"} })                             // seam per tipo
	Provide(fx.Annotate(func() string { return "app-runner" }, fx.ResultTags(`group:"runners"`))) // seam a gruppo

	var seenBusiness string
	var seen []string
	ModuleClosed("batch", func() {
		Provide(fx.Annotate(func() string { return "framework-runner" }, fx.ResultTags(`group:"runners"`)))
		Invoke(fx.Annotate(func(b *svcConfig, rs []string) {
			seenBusiness, seen = b.Url, rs
		}, fx.ParamTags(``, `group:"runners"`)))
	})

	app := fx.New(provides(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New error: %v", err)
	}
	startStop(t, app)
	if seenBusiness != "business" {
		t.Fatalf("seam per tipo = %q, want business", seenBusiness)
	}
	if len(seen) != 2 {
		t.Fatalf("gruppo visto dal modulo chiuso = %v, want 2 membri (root + modulo)", seen)
	}
}

// TestPrivateSupplyNoCollision documenta due comportamenti di dig su cui il design NON poggia, ma
// che cambiano rispetto a prima: due moduli fratelli possono supplire lo stesso tipo senza
// duplicate provide, e un Supply a root convive con l'omonimo privato di un modulo (che vince
// per i suoi costruttori).
func TestPrivateSupplyNoCollision(t *testing.T) {
	resetLists()
	var inA, inB, atRoot string
	Supply(&svcConfig{Url: "root"})
	Module("a", func() {
		Supply(&svcConfig{Url: "a"})
		Invoke(func(c *svcConfig) { inA = c.Url })
	})
	Module("b", func() {
		Supply(&svcConfig{Url: "b"})
		Invoke(func(c *svcConfig) { inB = c.Url })
	})
	Invoke(func(c *svcConfig) { atRoot = c.Url })

	app := fx.New(provides(), invokes(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New error: %v", err)
	}
	startStop(t, app)
	if inA != "a" || inB != "b" || atRoot != "root" {
		t.Fatalf("visibilità = a:%q b:%q root:%q, want a:\"a\" b:\"b\" root:\"root\"", inA, inB, atRoot)
	}
}

func startStop(t *testing.T, app *fx.App) {
	t.Helper()
	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		t.Fatalf("start error: %v", err)
	}
	if err := app.Stop(ctx); err != nil {
		t.Fatalf("stop error: %v", err)
	}
}

// TestConfigureApp_SvuotaIlRegistro copre il §3.2 dell'analisi: le liste di registrazione sono
// variabili di package, quindi senza svuotarle una seconda fx.App nello stesso processo eredita
// le registrazioni della prima. Run e Start sono duali e un'app ne chiama una sola: il caso reale
// non è "Run dopo Start" ma **più app nello stesso processo**, cioè i test — dove `-count=2`
// riesegue lo stesso test e il suo Supply è ancora nella lista.
func TestConfigureApp_SvuotaIlRegistro(t *testing.T) {
	resetLists()
	t.Cleanup(resetLists)

	type svc struct{ n int }

	Supply(&svc{n: 1})
	Provide(func() string { return "x" })
	Module("un-modulo", func() { Supply(42) })

	if app := configureApp(); app.Err() != nil {
		t.Fatalf("la prima app deve costruirsi: %v", app.Err())
	}

	if provideslist != nil || invokelist != nil || supply != nil ||
		populatelist != nil || modulelist != nil || current != nil {
		t.Fatal("registro non svuotato da configureApp: la prossima app erediterebbe queste registrazioni")
	}

	// Senza lo svuotamento qui dig direbbe "cannot provide *svc: already provided".
	Supply(&svc{n: 2})
	app := configureApp()
	if err := app.Err(); err != nil {
		t.Fatalf("la seconda app non deve vedere le registrazioni della prima: %v", err)
	}
}

// --- core.Private: granularità dentro un Module aperto ---

// svcGear è l'ingranaggio di un servizio: nel caso reale è la driver.Factory di go-core-kafka, che
// serve al costruttore del producer e a nessun altro.
type svcGear struct{ N int }

type svcOther struct{ Gear *svcGear }

// TestPrivateProvideStaysInModule: in un Module aperto, ciò che è registrato dentro Private resta
// dentro — ma il resto continua a uscire. È la granularità che prima non esisteva: o tutto esportato
// (Module) o niente (ModuleClosed).
func TestPrivateProvideStaysInModule(t *testing.T) {
	t.Run("l'ingranaggio non è iniettabile a root", func(t *testing.T) {
		resetLists()
		Module("kafka-producer", func() {
			Private(func() { Provide(func() *svcGear { return &svcGear{N: 1} }) })
			Provide(func(g *svcGear) *svcConfig { return &svcConfig{Url: "producer"} })
		})
		Invoke(func(*svcGear) {})

		if err := fx.New(provides(), invokes(), fx.Options(modulelist...)).Err(); err == nil {
			t.Fatal("fx.New senza errore: il provide dentro Private è ancora iniettabile a root")
		}
	})

	t.Run("il servizio esce e vede il suo ingranaggio", func(t *testing.T) {
		resetLists()
		var got string
		Module("kafka-producer", func() {
			Private(func() { Provide(func() *svcGear { return &svcGear{N: 1} }) })
			Provide(func(g *svcGear) *svcConfig { return &svcConfig{Url: "producer"} })
		})
		Invoke(func(s *svcConfig) { got = s.Url })

		app := fx.New(provides(), invokes(), fx.Options(modulelist...))
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		startStop(t, app)
		if got != "producer" {
			t.Fatalf("provide esportato del modulo = %q, want producer", got)
		}
	})
}

// TestPrivateNoCollisionTraFratelli è la ragione per cui Private esiste: due moduli che hanno lo
// stesso ingranaggio possono convivere nello stesso processo. Esportandolo, il secondo darebbe
// "already provided" — ed è il caso di due ProducerModule, o di un ProducerModule accanto a un
// Module con consumer.
func TestPrivateNoCollisionTraFratelli(t *testing.T) {
	resetLists()
	var seenA, seenB int
	Module("producer-a", func() {
		Private(func() { Provide(func() *svcGear { return &svcGear{N: 1} }) })
		Provide(func(g *svcGear) *svcConfig { return &svcConfig{Url: "a"} })
		Invoke(func(g *svcGear) { seenA = g.N })
	})
	Module("producer-b", func() {
		Private(func() { Provide(func() *svcGear { return &svcGear{N: 2} }) })
		Provide(func(g *svcGear) *svcOther { return &svcOther{Gear: g} })
		Invoke(func(g *svcGear) { seenB = g.N })
	})

	app := fx.New(provides(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New error: %v", err)
	}
	startStop(t, app)
	if seenA != 1 || seenB != 2 {
		t.Fatalf("ogni modulo deve vedere il PROPRIO ingranaggio: a=%d (want 1), b=%d (want 2)", seenA, seenB)
	}
}

// TestPrivateFuoriDaModulePanica: a root non esiste uno scope da cui nascondersi, quindi Private non
// avrebbe nulla da fare — e non farlo in silenzio sarebbe la risposta peggiore.
func TestPrivateFuoriDaModulePanica(t *testing.T) {
	resetLists()
	defer func() {
		if recover() == nil {
			t.Fatal("Private a root non ha panicato")
		}
	}()
	Private(func() { Provide(func() *svcGear { return &svcGear{} }) })
}

// Tipi di prova per lo smoke test dei provider nominati.
type namedClient struct{ id string }

type iNamedClient interface{ ID() string }

func (c *namedClient) ID() string { return c.id }

func newNamedClient(id string) *namedClient { return &namedClient{id: id} }

// resetLists azzera lo stato package-level tra i sotto-test.
// resetLists delega a resetRegistry: l'elenco delle liste sta in un posto solo (modules.go),
// altrimenti aggiungere una lista e dimenticarla qui darebbe test che si contaminano fra loro.
func resetLists() { resetRegistry() }

func TestProvideNamed(t *testing.T) {
	t.Run("named result resolved by name", func(t *testing.T) {
		resetLists()

		Provide(func() string { return "mngr-id" }) // consumato da newNamedClient
		ProvideNamed(newNamedClient, "mngr")

		var got *namedClient
		app := fx.New(
			provides(),
			fx.Invoke(fx.Annotate(func(c *namedClient) { got = c }, fx.ParamTags(`name:"mngr"`))),
		)
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		if got == nil || got.id != "mngr-id" {
			t.Fatalf("expected named client resolved, got %+v", got)
		}
	})

	t.Run("as + named combined", func(t *testing.T) {
		resetLists()

		Provide(func() string { return "primary-id" })
		ProvideAsNamed[iNamedClient](newNamedClient, "primary")

		var got iNamedClient
		app := fx.New(
			provides(),
			fx.Invoke(fx.Annotate(func(c iNamedClient) { got = c }, fx.ParamTags(`name:"primary"`))),
		)
		if err := app.Err(); err != nil {
			t.Fatalf("fx.New error: %v", err)
		}
		if got == nil || got.ID() != "primary-id" {
			t.Fatalf("expected named interface resolved, got %+v", got)
		}
	})
}

type fakeDep struct{ name string }

// testReg è il valore registrato nel value group, l'equivalente di una HandlerRegistration/TaskRunner.
type testReg struct {
	Owner  string
	Target any
}

const testGroup = "core_test_regs"

type regParams struct {
	In
	Regs []testReg `group:"core_test_regs"`
}

func synthFor[T any](t *testing.T, owner string, props properties.Properties, group string) any {
	t.Helper()
	ctor, err := synthCtor(reflect.TypeFor[T](), reflect.TypeFor[testReg](), group, owner, props,
		func(ptr any) any { return testReg{Owner: owner, Target: ptr} })
	if err != nil {
		t.Fatalf("synthCtor: %v", err)
	}
	return ctor
}

// depsObject è una dipendenza che è essa stessa un param object fx (caso "gruppo di dipendenze
// embeddato"): dig deve costruirla come param object ANNIDATO anche se nella struct sintetica il campo
// non è più embeddato.
type depsObject struct {
	In
	Dep *fakeDep
}

// taggedTarget è la forma raccomandata: dipendenze taggate `inject:`/`from:`, properties `prop:`,
// campi di lavorazione senza tag (che dig non deve vedere). Nessun core.In.
type taggedTarget struct {
	Dep    *fakeDep   `inject:""`
	Nested depsObject `inject:""`

	Collection string        `prop:"collection" validate:"required"`
	BatchLimit int           `prop:"batch-limit" default:"100"`
	Timeout    time.Duration `prop:"timeout" default:"5s"`

	Scratch []byte // lavorazione: esportato ma senza tag → dig non lo vede
	privato string // lavorazione non esportata
}

func TestSynthCtor_InjectsDepsAndBindsProps(t *testing.T) {
	var got *taggedTarget
	app := fx.New(
		fx.NopLogger,
		fx.Supply(&fakeDep{name: "svc"}),
		fx.Provide(synthFor[taggedTarget](t, `test: task "import"`,
			properties.Properties{"collection": "events", "batch-limit": 200}, testGroup)),
		fx.Invoke(func(p regParams) {
			if len(p.Regs) != 1 {
				t.Fatalf("attesa 1 registration nel gruppo, ottenuto %d", len(p.Regs))
			}
			got = p.Regs[0].Target.(*taggedTarget)
		}),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("il grafo fx deve costruirsi senza tag dig sui campi prop: %v", err)
	}
	if got.Dep == nil || got.Dep.name != "svc" {
		t.Fatalf("dipendenza non iniettata: %+v", got)
	}
	if got.Nested.Dep == nil {
		t.Fatal("param object annidato non iniettato")
	}
	if got.Collection != "events" || got.BatchLimit != 200 || got.Timeout != 5*time.Second {
		t.Fatalf("properties non mappate: %+v", got)
	}
	if got.Scratch != nil || got.privato != "" {
		t.Fatalf("i campi di lavorazione devono restare a zero: %+v", got)
	}
}

// Un campo esportato senza tag NON deve essere richiesto a dig: se lo fosse, questo grafo (che non
// fornisce alcuna string) non si costruirebbe.
func TestSynthCtor_UntaggedFieldIsNotADependency(t *testing.T) {
	type target struct {
		Dep     *fakeDep `inject:""`
		Scratch string   // lavorazione
	}
	app := fx.New(
		fx.NopLogger,
		fx.Supply(&fakeDep{name: "svc"}),
		fx.Provide(synthFor[target](t, "test", nil, testGroup)),
		fx.Invoke(func(p regParams) {
			if got := p.Regs[0].Target.(*target); got.Scratch != "" {
				t.Fatalf("campo di lavorazione valorizzato: %q", got.Scratch)
			}
		}),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("un campo senza tag non deve entrare nel grafo: %v", err)
	}
}

// `inject:"nome"` → name:"nome" per dig.
func TestSynthCtor_NamedDependency(t *testing.T) {
	type target struct {
		Primary *fakeDep `inject:"primary"`
	}
	var got *target
	app := fx.New(
		fx.NopLogger,
		fx.Supply(fx.Annotated{Name: "primary", Target: &fakeDep{name: "p"}}),
		fx.Supply(fx.Annotated{Name: "secondary", Target: &fakeDep{name: "s"}}),
		fx.Provide(synthFor[target](t, "test", nil, testGroup)),
		fx.Invoke(func(p regParams) { got = p.Regs[0].Target.(*target) }),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("dipendenza named non risolta: %v", err)
	}
	if got.Primary.name != "p" {
		t.Fatalf("iniettata la dipendenza sbagliata: %+v", got.Primary)
	}
}

// `from:"gruppo"` → group:"gruppo" per dig.
func TestSynthCtor_ValueGroupDependency(t *testing.T) {
	type target struct {
		Hooks []*fakeDep `from:"test_hooks"`
	}
	var got *target
	app := fx.New(
		fx.NopLogger,
		fx.Provide(fx.Annotate(func() *fakeDep { return &fakeDep{name: "h1"} }, fx.ResultTags(`group:"test_hooks"`))),
		fx.Provide(fx.Annotate(func() *fakeDep { return &fakeDep{name: "h2"} }, fx.ResultTags(`group:"test_hooks"`))),
		fx.Provide(synthFor[target](t, "test", nil, testGroup)),
		fx.Invoke(func(p regParams) { got = p.Regs[0].Target.(*target) }),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("value group non risolto: %v", err)
	}
	if len(got.Hooks) != 2 {
		t.Fatalf("attesi 2 contributori nel gruppo, ottenuto %d", len(got.Hooks))
	}
}

// `optional:"true"` resta opzionale: il grafo si costruisce anche senza provider.
func TestSynthCtor_OptionalDependency(t *testing.T) {
	type target struct {
		Dep *fakeDep `inject:"" optional:"true"`
	}
	var got *target
	app := fx.New(
		fx.NopLogger,
		fx.Provide(synthFor[target](t, "test", nil, testGroup)),
		fx.Invoke(func(p regParams) { got = p.Regs[0].Target.(*target) }),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("una dipendenza optional deve restare tale: %v", err)
	}
	if got.Dep != nil {
		t.Fatalf("atteso nil per la dipendenza optional non fornita: %+v", got.Dep)
	}
}

// Una dipendenza nil-abile mancante produce un errore NOSTRO, che nomina owner, campo e tipo (fx da
// solo direbbe "reflect.makeFuncStub": vedi il commento in synthCtor).
func TestSynthCtor_MissingDependency(t *testing.T) {
	type target struct {
		Dep *fakeDep `inject:""`
	}
	app := fx.New(
		fx.NopLogger,
		// nessun *fakeDep fornito
		fx.Provide(synthFor[target](t, `test: task "import"`, nil, testGroup)),
		fx.Invoke(func(regParams) {}),
	)
	err := app.Err()
	if err == nil {
		t.Fatal("atteso errore di dipendenza mancante")
	}
	for _, want := range []string{`task "import"`, "campo Dep", "*core.fakeDep"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("l'errore deve contenere %q: %v", want, err)
		}
	}
}

// Fallback: una dipendenza NON nil-abile (qui un param object annidato, che non possiamo rendere
// opzionale) resta obbligatoria per dig — l'errore è quello di fx, col tipo mancante.
func TestSynthCtor_MissingDependencyInNestedParamObject(t *testing.T) {
	type target struct {
		Nested depsObject `inject:""`
	}
	app := fx.New(fx.NopLogger,
		fx.Provide(synthFor[target](t, "test", nil, testGroup)),
		fx.Invoke(func(regParams) {}))
	if app.Err() == nil {
		t.Fatal("atteso errore: il param object annidato non è opzionale")
	}
	if !strings.Contains(app.Err().Error(), "*core.fakeDep") {
		t.Fatalf("l'errore fx deve nominare il tipo mancante: %v", app.Err())
	}
}

// Una property invalida fa fallire la costruzione del grafo: l'app non parte.
func TestSynthCtor_FailFastOnInvalidProperty(t *testing.T) {
	app := fx.New(
		fx.NopLogger,
		fx.Supply(&fakeDep{}),
		fx.Provide(synthFor[taggedTarget](t, "test", nil, testGroup)), // manca `collection`
		fx.Invoke(func(regParams) {}),
	)
	err := app.Err()
	if err == nil {
		t.Fatal("atteso fallimento della build del grafo fx")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Fatalf("l'errore fx deve riportare la property: %v", err)
	}
}

// core.In in una struct data a ProvideStruct è un errore: accettarlo lascerebbe passare le struct
// scritte per la vecchia semantica, con le dipendenze non taggate silenziosamente a nil.
type coreInTarget struct {
	In
	Dep *fakeDep
}

func TestSynthCtor_RejectsCoreIn(t *testing.T) {
	_, err := synthCtor(reflect.TypeFor[coreInTarget](), reflect.TypeFor[testReg](), testGroup, "test", nil,
		func(any) any { return testReg{} })
	if err == nil {
		t.Fatal("atteso errore per una struct che embedda core.In")
	}
	for _, want := range []string{"core.In", "inject"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("l'errore deve spiegare cosa fare (%q): %v", want, err)
		}
	}
}

// Senza value group il costruttore fornisce direttamente il risultato.
func TestSynthCtor_NoGroupProvidesResultDirectly(t *testing.T) {
	type target struct {
		Dep *fakeDep `inject:""`
	}
	var got testReg
	app := fx.New(
		fx.NopLogger,
		fx.Supply(&fakeDep{name: "svc"}),
		fx.Provide(synthFor[target](t, "test", nil, "")),
		fx.Invoke(func(r testReg) { got = r }),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("provide senza group fallito: %v", err)
	}
	if got.Target.(*target).Dep == nil {
		t.Fatal("dipendenza non iniettata")
	}
}

func TestSynthCtor_RejectsInvalidTagCombinations(t *testing.T) {
	cases := map[string]reflect.Type{
		"prop+inject": reflect.TypeFor[struct {
			X string `prop:"x" inject:""`
		}](),
		"from con nome": reflect.TypeFor[struct {
			X []string `inject:"nome" from:"gruppo"`
		}](),
		"from vuoto": reflect.TypeFor[struct {
			X []string `from:""`
		}](),
		"inject su campo non esportato": reflect.TypeFor[struct {
			x *fakeDep `inject:""`
		}](),
	}
	for name, typ := range cases {
		if _, err := synthCtor(typ, reflect.TypeFor[testReg](), testGroup, "test", nil, func(any) any { return testReg{} }); err == nil {
			t.Fatalf("%s: atteso errore di sintesi", name)
		}
	}
}

func TestSynthCtor_RejectsNonStruct(t *testing.T) {
	if _, err := synthCtor(reflect.TypeFor[int](), reflect.TypeFor[testReg](), testGroup, "test", nil, func(any) any { return testReg{} }); err == nil {
		t.Fatal("atteso errore per un tipo non struct")
	}
}

// Una struct non rappresentabile è un errore di programmazione: panic subito, al wiring.
func TestProvideStruct_PanicsOnNonStruct(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("atteso panic al wiring per un tipo non struct")
		}
	}()
	ProvideStruct(func(*int) testReg { return testReg{} }, "test", nil, testGroup)
}

type populated struct{ V string }

func TestPopulateDentroModuleVedeIPrivati(t *testing.T) {
	resetLists()
	var got *populated
	Module("mod", func() {
		Supply(&populated{V: "privato"})
		Populate(&got)
	})
	app := fx.New(provides(), invokes(), populates(), fx.Options(modulelist...))
	if err := app.Err(); err != nil {
		t.Fatalf("fx.New: %v", err)
	}
	if got == nil || got.V != "privato" {
		t.Fatalf("Populate dentro Module = %+v", got)
	}
}

func TestPrivate_PanicRipristinaLoScope(t *testing.T) {
	resetLists()
	func() {
		defer func() { _ = recover() }()
		Module("m", func() { Private(func() { panic("boom") }) })
	}()
	if current != nil || inPrivate {
		t.Fatalf("stato del registry non ripristinato: current=%v inPrivate=%v", current, inPrivate)
	}
}

func TestWaitContext(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { time.Sleep(10 * time.Millisecond); wg.Done() }()
	if !WaitContext(context.Background(), &wg) {
		t.Fatal("wg svuotato: atteso true")
	}

	var stuck sync.WaitGroup
	stuck.Add(1)
	defer stuck.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if WaitContext(ctx, &stuck) {
		t.Fatal("wg appeso: atteso false alla scadenza del context")
	}
}
