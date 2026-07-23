package core

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"slices"

	"github.com/ipfans/fxlogger"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

var provideslist []any
var Mode = os.Getenv("MODE")
var invokelist []fx.Option
var supply []fx.Option
var populatelist []any

// IsMode reports whether the current Mode is among the given modes.
// With no modes it returns true (i.e. "any mode"), coherently with the *If helpers.
func IsMode(acceptedmodes ...string) bool {
	if len(acceptedmodes) == 0 {
		return true
	}
	return slices.Contains(acceptedmodes, Mode)
}

// Provide registra un costruttore/valore. Se acceptedmodes è vuoto registra
// sempre; altrimenti solo se Mode è tra quelli indicati.
//
//	core.Provide(NewData)              // sempre
//	core.Provide(NewData, "batch")     // solo in mode "batch"
func Provide(provide any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		provideslist = append(provideslist, provide)
	}
}

// Supply registra un valore già istanziato. acceptedmodes opzionale come in Provide.
func Supply(value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		supply = append(supply, fx.Supply(value))
	}
}

// ProvideAs registra ctor annotandolo per essere fornito come l'interfaccia T,
// eliminando il boilerplate fx.Annotate(ctor, fx.As(new(T))). Il costruttore si
// passa nudo; l'interfaccia è il type parameter. acceptedmodes opzionale.
//
//	core.ProvideAs[IData](NewData)
//	core.ProvideAs[IData](NewData, engine.Batch, engine.Worker)
func ProvideAs[T any](ctor any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		Provide(fx.Annotate(ctor, fx.As(new(T))))
	}
}

// ProvideWith registra un costruttore insieme al valore (tipicamente il config)
// che consuma, in un'unica chiamata. acceptedmodes opzionale.
func ProvideWith(provide any, value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		Provide(provide)
		Supply(value)
	}
}

// ProvideAsWith è ProvideWith con il costruttore registrato come l'interfaccia T.
//
//	core.ProvideAsWith[IClient](NewService, &cfg.C, engine.Batch, engine.Worker, engine.Api)
func ProvideAsWith[T any](ctor any, value any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		ProvideAs[T](ctor)
		Supply(value)
	}
}

// Invoke registra una funzione eseguita all'avvio (side-effect). acceptedmodes opzionale.
func Invoke(invoke any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		invokelist = append(invokelist, fx.Invoke(invoke))
	}
}

// Populate registra un target per fx.Populate. acceptedmodes opzionale.
func Populate(top any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		populatelist = append(populatelist, top)
	}
}

func invokes() fx.Option {
	return fx.Options(invokelist...)
}

func populates() fx.Option {
	return fx.Populate(populatelist...)
}

func provides() fx.Option {
	supply = append(supply, fx.Provide(provideslist...))
	return fx.Options(supply...)
}

func Run() {
	app := configureApp()
	app.Run()
}

func Start(ctx context.Context) (*fx.App, error) {
	app := configureApp()
	err := app.Start(ctx)
	return app, err
}

func configureApp() *fx.App {

	fmt.Printf("%s\nVersion: %s\nSha: %s\nBuildDate: %s\nRuntime: %s\nOS: %s\nArch: %s\nNumCPU: %d\nGOMAXPROCS: %d\nGOMEMLIMIT=%s\n", string(Logo), BuildVersion, SHA, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), FormatBytes(debug.SetMemoryLimit(-1)))
	if Mode != "" {
		fmt.Printf("Mode: %s\n", Mode)
	}
	return fx.New(
		fx.WithLogger(fxlogger.WithZerolog(log.Logger)),
		provides(),
		populates(),
		invokes(),
	)
}
