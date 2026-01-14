package core

import (
	"fmt"
	"os"
	"strconv"
	"strings"

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
	AppConfig any `yaml:"config" mapstructure:"config" json:"config"`
}

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
		output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: DateTimeZoneFormat}
		output.FormatFieldName = func(i interface{}) string {
			return fmt.Sprintf("%s:", i)
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
