package core

import (
	"testing"

	"go.uber.org/fx"
)

// Config di prova: due sezioni, come le vuole rootConfig.
type bootAppConfig struct {
	Greeting string `mapstructure:"greeting"`
}

type bootMongoConfig struct {
	Host string `mapstructure:"host"`
}

type bootServicesConfig struct {
	Mongo bootMongoConfig `mapstructure:"mongo"`
}

const bootConfigYAML = `
log:
  ignore: true
config:
  app:
    greeting: ciao
  services:
    mongo:
      host: localhost
`

// saveIdentity ripristina le globali di identità/build toccate da Boot, così i test restano
// indipendenti l'uno dall'altro.
func saveIdentity(t *testing.T) {
	t.Helper()
	name, logo := AppName, Logo
	version, sha, date := BuildVersion, SHA, BuildDate
	t.Cleanup(func() {
		AppName, Logo = name, logo
		BuildVersion, SHA, BuildDate = version, sha, date
	})
}

func TestBoot(t *testing.T) {
	t.Run("ritorna la sezione services e fornisce quella app", func(t *testing.T) {
		resetLists()
		saveIdentity(t)

		svc := Boot[bootAppConfig, bootServicesConfig](App{
			Name:       "boot-test",
			ConfigFile: []byte(bootConfigYAML),
		})

		if svc == nil || svc.Mongo.Host != "localhost" {
			t.Fatalf("sezione services non decodificata: %+v", svc)
		}
		if AppName != "boot-test" {
			t.Fatalf("AppName = %q, atteso boot-test", AppName)
		}

		// La sezione app non torna al chiamante: Boot l'ha fornita a fx.
		var got *bootAppConfig
		app := fx.New(provides(), fx.Invoke(func(c *bootAppConfig) { got = c }))
		if err := app.Err(); err != nil {
			t.Fatalf("*bootAppConfig non risolvibile da fx: %v", err)
		}
		if got == nil || got.Greeting != "ciao" {
			t.Fatalf("sezione app non decodificata: %+v", got)
		}
	})

	t.Run("mode non ammesso", func(t *testing.T) {
		// Boot farebbe log.Fatal, quindi si verifica la condizione che usa.
		prev := Mode
		t.Cleanup(func() { Mode = prev })

		Mode = "PIPPO"
		if IsMode("API", "WORKER") {
			t.Fatal("MODE=PIPPO non deve essere ammesso da Modes=[API WORKER]")
		}
		Mode = "WORKER"
		if !IsMode("API", "WORKER") {
			t.Fatal("MODE=WORKER deve essere ammesso da Modes=[API WORKER]")
		}
		// Modes vuoto = app single-mode: nessuna validazione.
		Mode = "QUALSIASI"
		if !IsMode() {
			t.Fatal("con Modes vuoto non si valida nulla")
		}
	})
}

// TestBootValidaLeSezioni: la validazione delle regole non esprimibili coi tag non è più un
// core.Invoke dell'app — dimenticabile e dipendente dall'ordine di registrazione — ma un
// passaggio di Boot, su ENTRAMBE le sezioni e prima che la config entri nel grafo fx.
func TestBootValidaLeSezioni(t *testing.T) {
	resetLists()
	saveIdentity(t)

	// I contatori stanno in due variabili di package raggiunte dai metodi: Boot decodifica la
	// config in una struct sua, quindi non c'è un'istanza del test da ispezionare dopo.
	var app, services int
	bootValidateApp, bootValidateServices = &app, &services
	t.Cleanup(func() { bootValidateApp, bootValidateServices = nil, nil })

	Boot[bootValidateAppConfig, bootValidateServicesConfig](App{
		Name:       "boot-validate-test",
		ConfigFile: []byte(bootConfigYAML),
	})

	if app != 1 {
		t.Errorf("Validate() sulla sezione app chiamata %d volte, attesa 1", app)
	}
	if services != 1 {
		t.Errorf("Validate() sulla sezione services chiamata %d volte, attesa 1", services)
	}
}

// Le due sezioni del test sopra: implementano IValidate con receiver a puntatore, come lo
// trova Boot (che fa la type assertion su &cfg.App / &cfg.Services).
var bootValidateApp, bootValidateServices *int

type bootValidateAppConfig struct {
	Greeting string `mapstructure:"greeting"`
}

func (c *bootValidateAppConfig) Validate() error {
	if bootValidateApp != nil {
		*bootValidateApp++
	}
	return nil
}

type bootValidateServicesConfig struct {
	Mongo bootMongoConfig `mapstructure:"mongo"`
}

func (s *bootValidateServicesConfig) Validate() error {
	if bootValidateServices != nil {
		*bootValidateServices++
	}
	return nil
}

func TestFillBuildInfoNonSovrascriveIldflags(t *testing.T) {
	saveIdentity(t)

	BuildVersion, SHA, BuildDate = "1.2.3", "deadbeef", "2026-08-25T10:00:00+0200"
	fillBuildInfo()

	if BuildVersion != "1.2.3" || SHA != "deadbeef" || BuildDate != "2026-08-25T10:00:00+0200" {
		t.Fatalf("fillBuildInfo ha sovrascritto i valori del linker: %s %s %s", BuildVersion, SHA, BuildDate)
	}
}
