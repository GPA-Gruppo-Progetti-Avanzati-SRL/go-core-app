package core

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"os"
	"reflect"
	"strconv"
	"strings"
)

func Run[T ITaskRunner](runner T, shutdowner fx.Shutdowner) {

	go func() {
		log.Info().Msgf("Executing")
		runner.Execute()
		log.Info().Msg("Stopping")
		shutdowner.Shutdown()
	}()
}

type ITaskRunner interface {
	Execute()
}

var Task = &cobra.Command{}

var TaskConfig any

func Execute[T ITaskRunner]() {
	autoDefineFlags()
	Task.Flags().IntP("log", "l", 1, "level of logging: -1=trace, 0=debug, 1=info, 2=warn, 3=error")
	Task.Use = AppName
	Task.Version = BuildVersion
	Task.Run = func(cmd *cobra.Command, args []string) {
		//TaskConfig(cmd, args)
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			log.Fatal().Err(err).Msg("Error binding flags")
		}
		if err := viper.Unmarshal(&TaskConfig); err != nil {
			log.Fatal().Err(err).Msg("Error Unmarshal config")
		}
		logLevel, errll := cmd.Flags().GetInt("log")
		if errll != nil {
			log.Fatal().Err(errll).Msg("Error parsing log level")
		}
		Supply(TaskConfig)
		configureLog(logLevel)
		Invoke(Run[T])
		Start()
	}
	if err := Task.Execute(); err != nil {
		fmt.Println(err)
	}

}

func configureLog(logLevel int) {

	lvl, err := zerolog.ParseLevel(strings.ToLower(strconv.Itoa(logLevel)))
	if err != nil {
		log.Fatal().Err(err).Msg("Error parsing log level")
	}
	zerolog.SetGlobalLevel(lvl)

	zerolog.TimeFieldFormat = DateTimeZoneFormat
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: DateTimeZoneFormat}
	output.FormatFieldName = func(i interface{}) string {
		return fmt.Sprintf("%s:", i)
	}
	log.Logger = zerolog.New(output).With().Timestamp().Logger()

}

type FlagDefinition struct {
	name  string
	short string
	val   reflect.Value
	usage string
	field reflect.StructField
}

func autoDefineFlags() {
	val := reflect.ValueOf(TaskConfig).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		flagDef := FlagDefinition{
			name:  field.Tag.Get("mapstructure"),
			short: field.Tag.Get("short"),
			val:   val.Field(i),
			usage: field.Tag.Get("usage"),
			field: field,
		}

		addFlag(flagDef)

		if required := field.Tag.Get("required"); required == "true" {
			Task.MarkFlagRequired(flagDef.name)
		}
	}
}

func addFlag(def FlagDefinition) {
	flags := Task.Flags()

	switch def.field.Type.Kind() {
	case reflect.Int:
		value := def.val.Interface().(int)
		if def.short != "" {
			flags.IntP(def.name, def.short, value, def.usage)
		} else {
			flags.Int(def.name, value, def.usage)
		}
	case reflect.Bool:
		value := def.val.Interface().(bool)
		if def.short != "" {
			flags.BoolP(def.name, def.short, value, def.usage)
		} else {
			flags.Bool(def.name, value, def.usage)
		}
	case reflect.String:
		value := def.val.Interface().(string)
		if def.short != "" {
			flags.StringP(def.name, def.short, value, def.usage)
		} else {
			flags.String(def.name, value, def.usage)
		}
	case reflect.Slice, reflect.Array:
		if slice, ok := def.val.Interface().([]string); ok {
			if def.short != "" {
				flags.StringSliceVarP(&slice, def.name, def.short, slice, def.usage)
			} else {
				flags.StringSliceVar(&slice, def.name, slice, def.usage)
			}
		} else {
			log.Error().Msgf("Campo %s non è di tipo []string", def.name)
		}
	}
}
