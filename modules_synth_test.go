package core

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/fx"
)

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

func synthFor[T any](t *testing.T, owner string, props Properties, group string) any {
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
			Properties{"collection": "events", "batch-limit": 200}, testGroup)),
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

// Modalità legacy: una struct che embedda core.In mantiene la semantica storica — ogni campo esportato
// non-`prop:` è una dipendenza e i tag dig sono ricopiati verbatim.
type legacyTarget struct {
	In
	Dep        *fakeDep
	Optional   *fakeDep `name:"secondary" optional:"true"`
	Collection string   `prop:"collection" default:"eventi"`
}

func TestSynthCtor_LegacyCoreInSemantics(t *testing.T) {
	var got *legacyTarget
	app := fx.New(
		fx.NopLogger,
		fx.Supply(&fakeDep{name: "svc"}),
		fx.Provide(synthFor[legacyTarget](t, "test", nil, testGroup)),
		fx.Invoke(func(p regParams) { got = p.Regs[0].Target.(*legacyTarget) }),
	)
	if err := app.Err(); err != nil {
		t.Fatalf("la semantica legacy deve continuare a funzionare: %v", err)
	}
	if got.Dep == nil || got.Dep.name != "svc" {
		t.Fatalf("dipendenza legacy senza tag non iniettata: %+v", got)
	}
	if got.Optional != nil {
		t.Fatal("i tag dig legacy (name+optional) devono essere preservati")
	}
	if got.Collection != "eventi" {
		t.Fatalf("property non mappata in legacy: %q", got.Collection)
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
