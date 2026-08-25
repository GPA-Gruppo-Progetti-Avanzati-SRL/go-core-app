package core

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

// App descrive l'applicazione: i pochi fatti che il framework non può dedurre da sé.
//
// Non ha campi per versione/SHA/data di build: i ldflags puntano già alle var di questo package
// (-X github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app.BuildVersion=...), quindi
// BuildVersion & co. sono valorizzate dal linker. Farle assegnare da qui azzererebbe il valore
// linkato quando l'app non le ridichiara.
type App struct {
	Name         string   // -> AppName: Ambit degli errori, capabilities, service del router
	Logo         []byte   // banner stampato da Run
	ConfigFile   []byte   // //go:embed config.yml (obbligatorio)
	ConfigEnvVar string   // nome della env var col path della config esterna
	Modes        []string // modes ammessi; vuoto = nessuna validazione (app single-mode)
}

// rootConfig è la radice del sottoalbero `config:` di ogni app GPA: due sole sezioni, `app` (la
// config applicativa) e `services` (la config dell'infrastruttura). Non si dichiara nell'app — le
// portano i type param di Boot — e non esce dalla libreria: l'app riceve la sola sezione services,
// la sola che le serve in main().
type rootConfig[A, S any] struct {
	App      A `yaml:"app"      mapstructure:"app"      json:"app"`
	Services S `yaml:"services" mapstructure:"services" json:"services"`
}

// Boot esegue il preambolo di ogni app GPA: identità e build info, lettura e validazione della
// config (che configura anche il logging), fail-fast su MODE non ammesso, Supply della sezione
// applicativa. Ritorna la config dei servizi, quella che serve ai *.Module.
//
// Va chiamata come prima cosa in main(); poi il wiring, Run per ultimo. Su misconfig fa log.Fatal:
// l'app non parte, invece di girare con dei default silenziosi.
//
// I type param sono posizionali: PRIMA la config applicativa, POI quella dei servizi.
//
//	func main() {
//	    svc := core.Boot[api.Config, services.Config](core.App{
//	        Name: api.AppName, Logo: appLogo,
//	        ConfigFile: configFile, ConfigEnvVar: "APP_CONFIG_FILE_PATH",
//	    })
//
//	    coremongo.Module(&svc.Mongo)
//	    apiservices.Module(&svc.Api, apiservices.WithRoutes(routes.Register))
//
//	    core.Run(core.WithTracing())
//	}
//
// Invertire i type param non compila appena si usa il valore di ritorno (&svc.Mongo su una config
// applicativa non ha quel campo), quindi lo scambio è un errore a compile-time e non un silenzio a
// runtime.
//
// Un'app la cui config non ha la forma app/services usa direttamente ReadConfig.
func Boot[A, S any](app App) *S {
	AppName = app.Name
	if app.Logo != nil {
		Logo = app.Logo
	}
	fillBuildInfo()
	fmt.Printf("%s\nVersion: %s\nSha: %s\nBuildDate: %s\nRuntime: %s\nOS: %s\nArch: %s\nNumCPU: %d\nGOMAXPROCS: %d\nGOMEMLIMIT=%s\n", string(Logo), BuildVersion, SHA, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), FormatBytes(debug.SetMemoryLimit(-1)))
	if Mode != "" {
		fmt.Printf("Mode: %s\n", Mode)
	}
	cfg := new(rootConfig[A, S])
	if err := ReadConfig(string(app.ConfigFile), app.ConfigEnvVar, cfg); err != nil {
		log.Fatal().Err(err).Msg("failed to read config")
	}

	// Dopo ReadConfig: il logging è configurato, quindi il Fatal è formattato come tutti gli altri.
	if !IsMode(app.Modes...) {
		log.Fatal().Msgf("invalid MODE %q: expected one of %v", Mode, app.Modes)
	}

	// *A è la config applicativa: la iniettano data layer, business e task runner. È l'unico Supply
	// identico in tutte le app, quindi lo fa Boot e non compare più in main(). La sezione services
	// NON si fornisce: nessuno inietta l'aggregato, sono i singoli *.Module a prendersi il loro
	// sotto-config.
	Supply(&cfg.App)
	return &cfg.Services
}

// fillBuildInfo riempie SOLO i campi ancora vuoti a partire dai metadati che il toolchain
// incorpora nel binario: una build locale senza ldflags mostra comunque SHA e data. Non sovrascrive
// mai quello che ha messo il linker.
func fillBuildInfo() {
	if BuildVersion != "" && SHA != "" && BuildDate != "" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if BuildVersion == "" {
		BuildVersion = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if SHA == "" {
				SHA = s.Value
			}
		case "vcs.time":
			if BuildDate == "" {
				BuildDate = s.Value
			}
		}
	}
}
