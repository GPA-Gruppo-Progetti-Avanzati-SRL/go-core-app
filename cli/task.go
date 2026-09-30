// Package cli è il task runner a riga di comando: un binario one-shot che costruisce un comando
// cobra dai campi di TaskConfig, esegue un ITaskRunner dentro il grafo fx e termina. È l'unico
// package di go-core-app che porta cobra.
package cli

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func Exec[T ITaskRunner](runner T, shutdowner fx.Shutdowner) {

	go func() {
		log.Info().Msgf("Executing")
		runner.Execute()
		log.Info().Msg("Stopping")
		// Se lo shutdown non parte il task ha finito ma il processo resta su: senza questa riga
		// sarebbe un appeso senza spiegazione.
		if err := shutdowner.Shutdown(); err != nil {
			log.Error().Err(err).Msg("Shutdown dell'applicazione fallito: il processo resta attivo")
		}
	}()
}

type ITaskRunner interface {
	Execute()
}

var Task = &cobra.Command{}

var TaskConfig any

// Execute costruisce il comando dai campi di TaskConfig, lo esegue e termina. Un errore — flag non
// definibile, argomenti non validi, esecuzione fallita — fa uscire il processo con codice 1: prima
// veniva solo stampato, e uno scheduler o una pipeline vedevano un'uscita a 0, cioè un successo.
func Execute[T ITaskRunner]() {
	if err := execute[T](); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute[T ITaskRunner]() error {
	if err := autoDefineFlags(); err != nil {
		return err
	}
	Task.Flags().IntP("log", "l", 1, "level of logging: -1=trace, 0=debug, 1=info, 2=warn, 3=error")
	Task.Use = core.AppName
	Task.Version = core.BuildVersion
	Task.Run = func(cmd *cobra.Command, args []string) {
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
		core.Supply(TaskConfig)
		configureLog(logLevel)
		core.Invoke(Exec[T])
		core.Run()
	}
	return Task.Execute()
}

func configureLog(logLevel int) {

	lvl, err := zerolog.ParseLevel(strings.ToLower(strconv.Itoa(logLevel)))
	if err != nil {
		log.Fatal().Err(err).Msg("Error parsing log level")
	}
	zerolog.SetGlobalLevel(lvl)

	zerolog.TimeFieldFormat = core.DateTimeZoneFormat
	output := zerolog.ConsoleWriter{
		Out:             os.Stdout,
		TimeFormat:      core.DateTimeZoneFormat,
		FormatFieldName: func(i any) string { return fmt.Sprintf("%s:", i) },
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

// autoDefineFlags definisce un flag per ogni campo di TaskConfig, che dev'essere un puntatore a
// struct. Un TaskConfig nil o di un'altra forma, e un campo di un tipo che non sa rappresentare,
// sono errori: prima il primo panicava e il secondo spariva in silenzio, cioè un flag documentato
// dalla struct che la riga di comando non accettava.
func autoDefineFlags() error {
	rv := reflect.ValueOf(TaskConfig)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("cli: TaskConfig dev'essere un puntatore a struct, trovato %T", TaskConfig)
	}
	val := rv.Elem()
	typ := val.Type()

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		flagDef := FlagDefinition{
			name:  field.Tag.Get("mapstructure"),
			short: field.Tag.Get("short"),
			val:   val.Field(i),
			usage: field.Tag.Get("usage"),
			field: field,
		}
		if flagDef.name == "" {
			flagDef.name = strings.ToLower(field.Name)
		}

		if err := addFlag(flagDef); err != nil {
			return err
		}

		if required := field.Tag.Get("required"); required == "true" {
			if err := Task.MarkFlagRequired(flagDef.name); err != nil {
				return fmt.Errorf("cli: flag %q (campo %s) non marcabile come obbligatorio: %w", flagDef.name, field.Name, err)
			}
		}
	}
	return nil
}

var durationType = reflect.TypeFor[time.Duration]()

// addFlag definisce il flag del campo col suo tipo. I valori si leggono per Kind e non con una
// type assertion sul tipo esatto: un `type Livello int` passava il case reflect.Int e poi panicava
// su `.(int)`.
func addFlag(def FlagDefinition) error {
	flags := Task.Flags()
	name, short, usage := def.name, def.short, def.usage

	if def.field.Type == durationType {
		flags.DurationP(name, short, time.Duration(def.val.Int()), usage)
		return nil
	}
	switch def.field.Type.Kind() {
	case reflect.Int:
		flags.IntP(name, short, int(def.val.Int()), usage)
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		flags.Int64P(name, short, def.val.Int(), usage)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		flags.Uint64P(name, short, def.val.Uint(), usage)
	case reflect.Float32, reflect.Float64:
		flags.Float64P(name, short, def.val.Float(), usage)
	case reflect.Bool:
		flags.BoolP(name, short, def.val.Bool(), usage)
	case reflect.String:
		flags.StringP(name, short, def.val.String(), usage)
	case reflect.Slice:
		if def.field.Type.Elem().Kind() != reflect.String {
			return fmt.Errorf("cli: campo %s: slice di %s non supportata come flag (solo []string)", def.field.Name, def.field.Type.Elem())
		}
		cur := make([]string, def.val.Len())
		for i := range cur {
			cur[i] = def.val.Index(i).String()
		}
		flags.StringSliceP(name, short, cur, usage)
	default:
		return fmt.Errorf("cli: campo %s di tipo %s non supportato come flag", def.field.Name, def.field.Type)
	}
	return nil
}
