package core

import (
	"github.com/ipfans/fxlogger"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
	"os"
)

var provideslist []interface{}
var Mode = os.Getenv("MODE")
var invokelist []fx.Option

func ProvidesIf(provide interface{}, acceptedmodes ...string) {
	for _, item := range acceptedmodes {
		if item == Mode {
			provideslist = append(provideslist, provide)
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

func provides() fx.Option {
	return fx.Options(fx.Provide(provideslist...))
}

func Start() {
	fx.New(
		fx.WithLogger(fxlogger.WithZerolog(log.Logger)),
		provides(),
		invokes(),
	).Run()
}
