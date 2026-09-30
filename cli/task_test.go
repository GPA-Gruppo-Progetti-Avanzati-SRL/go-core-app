package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
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
