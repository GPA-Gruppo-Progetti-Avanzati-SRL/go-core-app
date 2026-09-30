package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/observability"
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/tpm-common/util"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	DateFormat         = "2006-01-02"
	DateTimeFormat     = "2006-01-02 15:04:05"
	DateTimeZoneFormat = "2006-01-02T15:04:05.999Z07:00"
)

type Config struct {
	Log struct {
		Ignore     bool
		Level      string
		EnableJSON bool
		Metric     bool
	}
	Metrics   observability.MetricsConfig `yaml:"metrics" mapstructure:"metrics" json:"metrics"`
	AppConfig any                         `yaml:"config" mapstructure:"config" json:"config"`
}

func ReadConfig(projectConfigFile, ConfigFileEnvVar string, appconfig any) error {

	configPath := os.Getenv(ConfigFileEnvVar)
	var cfgFileReader *strings.Reader
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			log.Info().Str("cfg-file-name", configPath).Msg("reading config")
			cfgContent, rerr := util.ReadFileAndResolveEnvVars(configPath)
			if rerr != nil {
				return rerr
			}
			cfgFileReader = strings.NewReader(string(cfgContent))

		} else {
			return fmt.Errorf("the %s env variable has been set but no file cannot be found at %s", ConfigFileEnvVar, configPath)
		}
	} else {
		log.Info().Msgf("The config path variable %s has not been set. Reverting to bundled configuration", ConfigFileEnvVar)
		cfgFileReader = strings.NewReader(util.ResolveConfigValueToString(projectConfigFile))

		// return nil, fmt.Errorf("the config path variable %s has not been set; please set", ConfigFileEnvVar)
	}

	var config = Config{
		AppConfig: appconfig,
	}

	viper.SetConfigType("yaml")

	viper.SetDefault("log.metric", true)

	// Default del server ops. Riproducono il comportamento storico: host vuoto significava
	// ":2112", cioè tutte le interfacce — necessario perché Prometheus scrapa l'IP del pod, non
	// 127.0.0.1. pprof resta spento salvo richiesta esplicita: la porta è raggiungibile da chi
	// arriva al processo, e /debug/pprof/profile è un CPU-burn mentre /heap può contenere segreti.
	viper.SetDefault("metrics.host", "0.0.0.0")
	viper.SetDefault("metrics.port", 2112)
	viper.SetDefault("metrics.pprof", false)
	viper.SetDefault("metrics.read-header-timeout", 5*time.Second)

	// Senza un default, un file che non nomina `log.level` arriva qui con la stringa vuota, che
	// zerolog.ParseLevel accetta come NoLevel: il livello globale finiva sopra Fatal, e ogni
	// log.Fatal successivo — MODE non ammesso, IValidate di Boot, quelli dell'app — usciva con
	// codice 1 senza stampare nulla.
	viper.SetDefault("log.level", "info")

	if verr := viper.ReadConfig(cfgFileReader); verr != nil {
		return fmt.Errorf("unable to read config: %w", verr)
	}
	if err := viper.Unmarshal(&config); err != nil {
		return fmt.Errorf("unable to decode config: %w", err)
	}

	// La sezione `metrics:` è consumata dalla libreria stessa (observability.NewServerMetrics), esattamente
	// come `log:` qui sotto: si deposita ora, mentre la struct è viva, perché ReadConfig la scarta.
	observability.SetMetricsConfig(config.Metrics)

	if !config.Log.Ignore {
		lvl, err := parseLogLevel(config.Log.Level)
		if err != nil {
			return err
		}
		zerolog.SetGlobalLevel(lvl)
	}

	// Il logger si ricostruisce da zero in entrambi i rami: agganciare l'hook a quello corrente lo
	// accumulerebbe a ogni ReadConfig, e ogni evento verrebbe contato una volta per chiamata.
	zerolog.TimeFieldFormat = DateTimeZoneFormat
	if !config.Log.EnableJSON {
		output := zerolog.ConsoleWriter{
			Out:             os.Stdout,
			TimeFormat:      DateTimeZoneFormat,
			FormatFieldName: func(i any) string { return fmt.Sprintf("%s:", i) },
		}
		log.Logger = zerolog.New(output).With().Timestamp().Logger()
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

	if config.Log.Metric {
		log.Logger = log.Logger.Hook(observability.SharedMetricLogHook())
	}

	if errValidate := ValidateStruct(config); errValidate != nil {
		// NON si logga la config: a questo punto i ${...} sono già risolti, quindi il dump
		// conterrebbe password e DSN in chiaro. L'errore nomina già i campi invalidi, che è
		// l'unica cosa che serve per correggere il file.
		return fmt.Errorf("error validating config: %w", errValidate)
	}

	return nil
}

// parseLogLevel accetta sia il nome del livello ("debug", "INFO") sia il suo valore numerico
// zerolog ("-1".."7"). Rifiuta NoLevel e Disabled scritti per nome, perché nessuno dei due è un
// livello: il primo è ciò che ParseLevel ritorna per la stringa vuota, il secondo spegne anche i
// Fatal. Chi vuole davvero il silenzio ha `log.ignore`.
func parseLogLevel(s string) (zerolog.Level, error) {
	if i, err := strconv.Atoi(s); err == nil {
		return zerolog.Level(i), nil
	}
	lvl, err := zerolog.ParseLevel(strings.ToLower(strings.TrimSpace(s)))
	if err != nil {
		return zerolog.NoLevel, fmt.Errorf("log.level %q: %w", s, err)
	}
	if lvl == zerolog.NoLevel || lvl == zerolog.Disabled {
		return zerolog.NoLevel, fmt.Errorf("log.level %q is not a level (use trace, debug, info, warn, error, fatal, panic)", s)
	}
	return lvl, nil
}
