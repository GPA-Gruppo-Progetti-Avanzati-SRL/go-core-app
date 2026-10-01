package cli

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

func resetTask(t *testing.T, cfg any) {
	t.Helper()
	prevTask, prevCfg := Task, TaskConfig
	t.Cleanup(func() { Task, TaskConfig = prevTask, prevCfg })
	Task, TaskConfig = &cobra.Command{}, cfg
}

type livello int

type tuttiITipi struct {
	Nome     string        `mapstructure:"nome" short:"n" usage:"il nome"`
	Conta    int           `mapstructure:"conta"`
	Livello  livello       `mapstructure:"livello"` // un tipo nominale: prima `.(int)` panicava
	Grande   int64         `mapstructure:"grande"`
	Senza    uint          `mapstructure:"senza"`
	Soglia   float64       `mapstructure:"soglia"`
	Attivo   bool          `mapstructure:"attivo"`
	Attesa   time.Duration `mapstructure:"attesa"`
	Tag      []string      `mapstructure:"tag"`
	Implicit string        // senza mapstructure: nome del campo in minuscolo
	interno  string
}

func TestAutoDefineFlags_TuttiITipi(t *testing.T) {
	resetTask(t, &tuttiITipi{Conta: 3, Livello: 2, Attesa: time.Second, Tag: []string{"a"}})
	if err := autoDefineFlags(); err != nil {
		t.Fatalf("autoDefineFlags: %v", err)
	}
	for _, name := range []string{"nome", "conta", "livello", "grande", "senza", "soglia", "attivo", "attesa", "tag", "implicit"} {
		if Task.Flags().Lookup(name) == nil {
			t.Errorf("flag %q non definito", name)
		}
	}
	if got := Task.Flags().Lookup("livello").DefValue; got != "2" {
		t.Errorf("default di livello = %q", got)
	}
	if got := Task.Flags().Lookup("attesa").DefValue; got != "1s" {
		t.Errorf("default di attesa = %q", got)
	}
	if Task.Flags().ShorthandLookup("n") == nil {
		t.Error("shorthand -n non definito")
	}
	_ = tuttiITipi{}.interno
}

func TestAutoDefineFlags_Errori(t *testing.T) {
	cases := map[string]any{
		"nil":             nil,
		"non puntatore":   tuttiITipi{},
		"puntatore a int": new(int),
		"tipo non supportato": &struct {
			M map[string]string `mapstructure:"m"`
		}{},
		"slice non di stringhe": &struct {
			N []int `mapstructure:"n"`
		}{},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			resetTask(t, cfg)
			if err := autoDefineFlags(); err == nil {
				t.Fatal("atteso errore, non un panic né un flag saltato in silenzio")
			} else if !strings.HasPrefix(err.Error(), "cli:") {
				t.Fatalf("errore senza contesto: %v", err)
			}
		})
	}
}

func TestAutoDefineFlags_Required(t *testing.T) {
	resetTask(t, &struct {
		Path string `mapstructure:"path" required:"true"`
	}{})
	if err := autoDefineFlags(); err != nil {
		t.Fatal(err)
	}
	if ann := Task.Flags().Lookup("path").Annotations[cobra.BashCompOneRequiredFlag]; len(ann) == 0 {
		t.Fatal("flag obbligatorio non marcato")
	}
}

// startedService segna l'esecuzione del proprio OnStart, come un client che si connette all'avvio.
type startedService struct{ started atomic.Bool }

type orderRunner struct {
	svc  *startedService
	seen chan bool
}

func (r orderRunner) Execute(context.Context) error { r.seen <- r.svc.started.Load(); return nil }

// Il task gira a grafo avviato: prima la goroutine partiva al momento dell'Invoke, PRIMA degli
// OnStart, e un task poteva usare un client non ancora connesso.
func TestExec_DopoGliOnStart(t *testing.T) {
	seen := make(chan bool, 1)
	app := fxtest.New(t,
		fx.Provide(func(lc fx.Lifecycle) *startedService {
			s := &startedService{}
			lc.Append(fx.Hook{OnStart: func(context.Context) error { s.started.Store(true); return nil }})
			return s
		}),
		fx.Provide(func(s *startedService) orderRunner { return orderRunner{svc: s, seen: seen} }),
		fx.Invoke(Exec[orderRunner]),
	)
	app.RequireStart()
	defer app.RequireStop()
	select {
	case started := <-seen:
		if !started {
			t.Fatal("il task è partito prima degli OnStart dei servizi")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("il task non è partito")
	}
}

type failingRunner struct{}

func (failingRunner) Execute(context.Context) error { return errors.New("export fallito") }

// Un task fallito chiude l'applicazione con 1: prima Execute non ritornava nulla, e il processo
// usciva con 0 anche quando il lavoro non era stato fatto.
func TestExec_ErroreDelTaskEsceConUno(t *testing.T) {
	app := fxtest.New(t,
		fx.Supply(failingRunner{}),
		fx.Invoke(Exec[failingRunner]),
	)
	app.RequireStart()
	defer app.RequireStop()
	select {
	case sig := <-app.Wait():
		if sig.ExitCode != 1 {
			t.Fatalf("exit code = %d, atteso 1", sig.ExitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("l'applicazione non si è chiusa")
	}
}
