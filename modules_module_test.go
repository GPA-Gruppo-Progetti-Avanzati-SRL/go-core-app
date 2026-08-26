package core

import (
	"context"
	"testing"

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
