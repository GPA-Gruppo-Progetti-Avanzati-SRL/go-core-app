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
	Task.Flags().IntP("log", "l", 2, "level of logging: -1=trace, 0=debug, 1=info, 2=warn, 3=error")
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

func autoDefineFlags() {
	val := reflect.ValueOf(TaskConfig).Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := field.Tag.Get("mapstructure")
		usage := field.Tag.Get("usage")
		required := field.Tag.Get("required")
		short := field.Tag.Get("short")
		if short != "" {
			switch field.Type.Kind() {
			case reflect.Int:
				Task.Flags().IntP(name, short, val.Field(i).Interface().(int), usage)
			case reflect.Bool:
				Task.Flags().BoolP(name, short, val.Field(i).Interface().(bool), usage)
			case reflect.String:
				Task.Flags().StringP(name, short, val.Field(i).Interface().(string), usage)
			}
		} else {
			switch field.Type.Kind() {
			case reflect.Int:
				Task.Flags().Int(name, val.Field(i).Interface().(int), usage)
			case reflect.Bool:
				Task.Flags().Bool(name, val.Field(i).Interface().(bool), usage)
			case reflect.String:
				Task.Flags().String(name, val.Field(i).Interface().(string), usage)
			}

		}

		if required == "true" {
			Task.MarkFlagRequired(name)
		}
	}
}
