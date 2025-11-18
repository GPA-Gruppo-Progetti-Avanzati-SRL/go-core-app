package core

import (
	"fmt"
	"os"
	"runtime"

	"github.com/ipfans/fxlogger"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

var provideslist []interface{}
var Mode = os.Getenv("MODE")
var invokelist []fx.Option
var supply []fx.Option
var populatelist []interface{}

func ProvidesIf(provide interface{}, acceptedmodes ...string) {
	for _, item := range acceptedmodes {
		if item == Mode {
			provideslist = append(provideslist, provide)
			return
		}
	}

}

func Supply(ifaces ...interface{}) {
	for _, iface := range ifaces {
		supply = append(supply, fx.Supply(iface))
	}
}

func SupplyIf(iface interface{}, acceptedmodes ...string) {
	for _, item := range acceptedmodes {
		if item == Mode {
			supply = append(supply, fx.Supply(iface))
			return
		}
	}
}

func ProvidesAndSupplyIf(provide interface{}, supply interface{}, acceptedmodes ...string) {

	for _, item := range acceptedmodes {
		if item == Mode {
			Provides(provide)
			Supply(supply)
			return
		}
	}

}

func Provides(methods ...interface{}) {
	for _, item := range methods {
		provideslist = append(provideslist, item)
	}
}

func Invoke(invoke interface{}) {
	invokelist = append(invokelist, fx.Invoke(invoke))
	return
}

func Populate(top interface{}) {
	populatelist = append(populatelist, top)
	return
}

func InvokeIf(invoke interface{}, acceptedmodes ...string) {
	for _, item := range acceptedmodes {
		if item == Mode {
			invokelist = append(invokelist, fx.Invoke(invoke))
			return
		}
	}
}

func invokes() fx.Option {
	return fx.Options(invokelist...)
}

func populates() fx.Option {
	return fx.Populate(provideslist...)
}

func provides() fx.Option {
	supply = append(supply, fx.Provide(provideslist...))
	return fx.Options(supply...)
}

func Start() {

	fmt.Printf("%s\nVersion: %s\nSha: %s\nBuildDate: %s\nRuntime: %s\nOS: %s\nArch: %s\n", string(Logo), BuildVersion, SHA, BuildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if Mode != "" {
		fmt.Printf("Mode: %s\n", Mode)

	}
	fx.New(
		fx.WithLogger(fxlogger.WithZerolog(log.Logger)),
		provides(),
		populates(),
		invokes(),
	).Run()
}
