package core

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/fx"
)

// Regressioni dei difetti corretti nella revisione di go-core-app del 2026-09-30.

func TestParseLogLevel(t *testing.T) {
	ok := map[string]zerolog.Level{"debug": zerolog.DebugLevel, " INFO ": zerolog.InfoLevel, "3": zerolog.ErrorLevel, "-1": zerolog.TraceLevel}
	for in, want := range ok {
		got, err := parseLogLevel(in)
		if err != nil || got != want {
			t.Errorf("parseLogLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	// "" era accettato come NoLevel: il livello globale finiva sopra Fatal e ogni log.Fatal
	// usciva senza stampare nulla.
	for _, in := range []string{"", "disabled", "verbose"} {
		if _, err := parseLogLevel(in); err == nil {
			t.Errorf("parseLogLevel(%q): atteso errore", in)
		}
	}
}

func keepLogLevel(t *testing.T) {
	t.Helper()
	prev := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })
}

func TestReadConfig_SenzaLogLevelResteInfo(t *testing.T) {
	keepLogLevel(t)
	zerolog.SetGlobalLevel(zerolog.Disabled)
	var cfg struct{}
	if err := ReadConfig("log:\n  metric: false\n", "CORE_TEST_NO_SUCH_ENV", &cfg); err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if got := zerolog.GlobalLevel(); got != zerolog.InfoLevel {
		t.Fatalf("livello globale = %v, atteso info", got)
	}
}

func TestReadConfig_RitornaGliErrori(t *testing.T) {
	keepLogLevel(t)
	var cfg struct{}
	if err := ReadConfig("log: [non: una: mappa", "CORE_TEST_NO_SUCH_ENV", &cfg); err == nil {
		t.Fatal("YAML malformato: atteso un errore restituito, non un Fatal")
	}

	var invalid struct {
		Name string `mapstructure:"name" validate:"required"`
	}
	err := ReadConfig("log:\n  ignore: true\nconfig:\n  other: 1\n", "CORE_TEST_NO_SUCH_ENV", &invalid)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("validazione fallita: atteso un errore che nomini il campo, got %v", err)
	}
}

func TestConcurrentN_ConcorrenzaNonPositiva(t *testing.T) {
	for _, c := range []int{0, -3} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			out, err := ConcurrentN([]int{1, 2, 3}, c, func(i int) (int, *ApplicationError) { return i * 2, nil })
			if err != nil || len(out) != 3 || out[2] != 6 {
				t.Errorf("concurrency=%d: out=%v err=%v", c, out, err)
			}
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("concurrency=%d: ConcurrentN bloccata", c)
		}
	}
}

func TestConcurrentN_PanicDiventaErrore(t *testing.T) {
	_, err := ConcurrentN([]int{1, 2}, 2, func(i int) (int, *ApplicationError) {
		if i == 2 {
			panic("boom")
		}
		return i, nil
	})
	if err == nil || err.Code != ErrConcurrentPanic {
		t.Fatalf("atteso %s, got %v", ErrConcurrentPanic, err)
	}
	if _, _, err := ConcurrentTwo(
		func() (int, *ApplicationError) { return 1, nil },
		func() (int, *ApplicationError) { panic("boom") },
	); err == nil || err.Code != ErrConcurrentPanic {
		t.Fatalf("ConcurrentTwo: atteso %s, got %v", ErrConcurrentPanic, err)
	}
}

type withTime struct {
	At  time.Time
	N   int
	Sub struct{ D time.Duration }
}

func TestInherit_StructOpacaEredita(t *testing.T) {
	g := withTime{At: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC), N: 5}
	var p withTime
	Inherit(&p, &g)
	if !p.At.Equal(g.At) {
		t.Fatalf("time.Time non ereditato: %v", p.At)
	}
	if IsZeroStruct(withTime{At: g.At}) {
		t.Fatal("IsZeroStruct: un time.Time valorizzato conta come scritto")
	}
}

func TestIsZeroStruct_NegativiComeInherit(t *testing.T) {
	// Inherit sovrascrive -1 col valore globale: per IsZeroStruct è quindi "non scritto".
	if !IsZeroStruct(withTime{N: -1}) {
		t.Fatal("IsZeroStruct(N=-1) = false, ma Inherit tratta -1 come non valorizzato")
	}
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

func TestApplicationError_ErrorSenzaMessaggio(t *testing.T) {
	e := TechnicalError().WithCode("X-1")
	if e.Error() != "X-1" {
		t.Fatalf("Error() = %q, atteso il codice", e.Error())
	}
	e.Log(nil) // non deve panicare
	if !errors.Is(TechnicalError().WithCause(errors.ErrUnsupported), errors.ErrUnsupported) {
		t.Fatal("catena Unwrap interrotta")
	}
}

func TestHttpClient_HaUnTimeout(t *testing.T) {
	if c := GenerateHttpClientWithInstrumentation("svc"); c.Timeout != DefaultHttpClientTimeout {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
	if c := GenerateHttpClientWithInstrumentation("svc", time.Second); c.Timeout != time.Second {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
}

func TestValidateStruct_MessaggioTradotto(t *testing.T) {
	var v struct {
		Name string `mapstructure:"name" validate:"required"`
	}
	err := ValidateStruct(v)
	if err == nil {
		t.Fatal("atteso errore")
	}
	if strings.Contains(err.Message, "Key: '") || strings.Contains(err.Message, "[") {
		t.Fatalf("messaggio non tradotto o con le parentesi della slice: %q", err.Message)
	}
}
