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
