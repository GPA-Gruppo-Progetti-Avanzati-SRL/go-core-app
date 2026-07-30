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
