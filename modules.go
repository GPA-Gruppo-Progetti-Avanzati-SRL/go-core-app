package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/ipfans/fxlogger"
	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

// init imposta GOMEMLIMIT dal limite del cgroup, ma SOLO se il processo gira
// effettivamente dentro un cgroup con un limite di memoria (tipicamente in
// container/k8s). Fuori da un cgroup — o su sistemi che non li supportano, es.
// macOS in sviluppo locale — la libreria non deve emettere l'errore
// "failed to set GOMEMLIMIT: cgroups is not supported on this system".
// Per ottenerlo avvolgiamo FromCgroup convertendo "non in un cgroup" in
// memlimit.ErrNoLimit, che il framework tratta come skip silenzioso.
func init() {
	memlimit.SetGoMemLimitWithOpts(
		memlimit.WithProvider(cgroupOnlyProvider),
		memlimit.WithLogger(slog.Default()),
	)
}

// cgroupOnlyProvider ritorna il limite del cgroup; se il processo non è dentro
// un cgroup (ErrNoCgroup) o il sistema non li supporta (ErrCgroupsNotSupported)
// ritorna memlimit.ErrNoLimit invece di propagare l'errore.
func cgroupOnlyProvider() (uint64, error) {
	limit, err := memlimit.FromCgroup()
	if errors.Is(err, memlimit.ErrNoCgroup) || errors.Is(err, memlimit.ErrCgroupsNotSupported) {
		return 0, memlimit.ErrNoLimit
	}
	return limit, err
}

var provideslist []interface{}
var Mode = os.Getenv("MODE")
var invokelist []fx.Option
var supply []fx.Option
var populatelist []interface{}

// IsMode reports whether the current Mode is among the given modes.
// With no modes it returns true (i.e. "any mode"), coherently with the *If helpers.
func IsMode(acceptedmodes ...string) bool {
	if len(acceptedmodes) == 0 {
		return true
	}
	for _, item := range acceptedmodes {
		if item == Mode {
			return true
		}
	}
	return false
}

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
