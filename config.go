package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
	Metrics   MetricsConfig `yaml:"metrics" mapstructure:"metrics" json:"metrics"`
	AppConfig any           `yaml:"config" mapstructure:"config" json:"config"`
}

// MetricsConfig configura il server ops di NewServerMetrics (/metrics, /health e — solo se
// richiesto — /debug/pprof/*). La sezione `metrics:` è facoltativa: i default di viperDefaults
// riproducono esattamente il comportamento storico (0.0.0.0:2112, pprof spento), quindi una
// config che non la nomina non cambia di una virgola.
type MetricsConfig struct {
	Host              string        `yaml:"host" mapstructure:"host" json:"host"`
	Port              int           `yaml:"port" mapstructure:"port" json:"port"`
	Pprof             bool          `yaml:"pprof" mapstructure:"pprof" json:"pprof"`
	ReadHeaderTimeout time.Duration `yaml:"read-header-timeout" mapstructure:"read-header-timeout" json:"read-header-timeout"`
}

// metricsConfig è la sezione `metrics:` letta da ReadConfig. Sta in una var di package per la
// stessa ragione per cui ci stanno Mode, AppName e BuildVersion: NewServerMetrics è un invoke fx
// senza parametri di config, e passargliela cambierebbe la firma di WithServerMetrics — cioè
// costringerebbe ogni app a scrivere un argomento per una sezione che quasi nessuna valorizza.
var metricsConfig MetricsConfig

// MetricsSettings ritorna la configurazione del server ops effettivamente in uso (default
// compresi). Esportata perché è l'unico modo, per un'app o un test, di sapere su quale indirizzo
// il server è stato messo in ascolto senza reimplementare i default.
func MetricsSettings() MetricsConfig { return metricsConfig }

func ReadConfig(projectConfigFile, ConfigFileEnvVar string, appconfig any) error {

	configPath := os.Getenv(ConfigFileEnvVar)
	var cfgFileReader *strings.Reader
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			log.Info().Str("cfg-file-name", configPath).Msg("reading config")
			cfgContent, rerr := util.ReadFileAndResolveEnvVars(configPath)
			if rerr != nil {
				return err
			} else {
				cfgFileReader = strings.NewReader(string(cfgContent))
			}

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

	verr := viper.ReadConfig(cfgFileReader)

	if verr != nil {
		log.Fatal().Msgf("unable to read config, %v", verr)
	}
	err := viper.Unmarshal(&config)
	if err != nil {
		log.Fatal().Msgf("unable to decode into struct, %v", err)
	}

	if err != nil {
		return err
	}

	// La sezione `metrics:` è consumata dalla libreria stessa (NewServerMetrics), esattamente come
	// `log:` qui sotto: si deposita ora, mentre la struct è viva, perché ReadConfig la scarta.
	metricsConfig = config.Metrics

	if !config.Log.Ignore {
		i, err := strconv.Atoi(config.Log.Level)
		if err != nil {
			lvl, err := zerolog.ParseLevel(strings.ToLower(config.Log.Level))
			if err != nil {
				return err
			}
			zerolog.SetGlobalLevel(lvl)
		} else {
			zerolog.SetGlobalLevel(zerolog.Level(i))
		}
	}

	if !config.Log.EnableJSON {
		zerolog.TimeFieldFormat = DateTimeZoneFormat
		output := zerolog.ConsoleWriter{
			Out:             os.Stdout,
			TimeFormat:      DateTimeZoneFormat,
			FormatFieldName: func(i any) string { return fmt.Sprintf("%s:", i) },
		}
		log.Logger = zerolog.New(output).With().Timestamp().Logger()
	} else {
		zerolog.TimeFieldFormat = DateTimeZoneFormat
	}

	if config.Log.Metric {
		metricHook := &MetricLogHook{}
		metricHook.Init()

		log.Logger = log.Logger.Hook(metricHook)
	}

	if errValidate := ValidateStruct(config); errValidate != nil {
		log.Err(errValidate).Msgf("%v", config)
		log.Fatal().Err(errValidate).Msgf("error validating config, %v", errValidate)
	}

	return nil
}
