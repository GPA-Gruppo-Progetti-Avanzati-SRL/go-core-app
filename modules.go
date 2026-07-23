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

func ProvideIf(provide any, acceptedmodes ...string) {
	if slices.Contains(acceptedmodes, Mode) {
		provideslist = append(provideslist, provide)
		return
	}

}

func Supply(ifaces ...any) {
	for _, iface := range ifaces {
		supply = append(supply, fx.Supply(iface))
	}
}

func SupplyIf(iface any, acceptedmodes ...string) {
	if slices.Contains(acceptedmodes, Mode) {
		supply = append(supply, fx.Supply(iface))
		return
	}
}

func ProvideAndSupplyIf(provide any, supply any, acceptedmodes ...string) {

	if slices.Contains(acceptedmodes, Mode) {
		Provides(provide)
		Supply(supply)
		return
	}

}

func Provides(methods ...any) {
	for _, item := range methods {
		provideslist = append(provideslist, item)
	}
}

// ProvideInterface registra ctor annotandolo per essere fornito come
// l'interfaccia T, eliminando il boilerplate fx.Annotate(ctor, fx.As(new(T))).
// Il costruttore si passa nudo; l'interfaccia è il type parameter.
//
//	core.ProvideInterface[IData](NewData)
func ProvideInterface[T any](ctor any) {
	Provides(fx.Annotate(ctor, fx.As(new(T))))
}

// ProvideInterfaceIf è ProvideInterface, gated sul Mode corrente (usa IsMode).
func ProvideInterfaceIf[T any](ctor any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		ProvideInterface[T](ctor)
	}
}

// ProvideInterfaceAndSupply registra ctor come interfaccia T e fa Supply del valore dato.
func ProvideInterfaceAndSupply[T any](ctor any, supply any) {
	ProvideInterface[T](ctor)
	Supply(supply)
}

// ProvideInterfaceAndSupplyIf è ProvideInterfaceAndSupply, gated sul Mode corrente (usa IsMode).
//
//	core.ProvideInterfaceAndSupplyIf[IClient](NewService, &cfg.C, engine.Batch, engine.Worker, engine.Api)
func ProvideInterfaceAndSupplyIf[T any](ctor any, supply any, acceptedmodes ...string) {
	if IsMode(acceptedmodes...) {
		ProvideInterfaceAndSupply[T](ctor, supply)
	}
}

func Invoke(invoke any) {
	invokelist = append(invokelist, fx.Invoke(invoke))
	return
}

func Populate(top any) {
	populatelist = append(populatelist, top)
	return
}

func InvokeIf(invoke any, acceptedmodes ...string) {
	if slices.Contains(acceptedmodes, Mode) {
		invokelist = append(invokelist, fx.Invoke(invoke))
		return
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
