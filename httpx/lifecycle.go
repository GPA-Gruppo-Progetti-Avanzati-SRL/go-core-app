// Package httpx raccoglie ciò che serve attorno a net/http nelle librerie go-core: il ciclo di
// vita fx di un *http.Server (ServeOnLifecycle) e il client strumentato OTel con timeout di
// default (GenerateHttpClientWithInstrumentation).
package httpx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"
	"go.uber.org/fx"
)

// ServeOnLifecycle aggancia un *http.Server al ciclo di vita fx: è l'unico modo in cui le librerie
// go-core mettono in ascolto un server HTTP (il server ops di WithServerMetrics, l'API di
// go-core-api), perché le copie che c'erano prima avevano già preso decisioni diverse su cosa
// succede quando il server muore.
//
//   - Il listener si apre DENTRO OnStart e l'errore è restituito: una porta occupata fa fallire
//     l'avvio invece di finire in una goroutine che nessuno osserva.
//   - Se Serve termina con un errore che non è l'arresto ordinato, il processo ESCE (Shutdowner
//     con codice 1). Un server morto in un processo vivo è il peggiore dei due stati: l'unica
//     probe che se ne accorgerebbe è /health, che è servita dallo stesso server — con l'API, la
//     sola cosa che lo diceva era una riga di log.
//   - OnStop chiama Shutdown col context dell'hook, quindi l'attesa delle richieste in volo è
//     limitata da fx.StopTimeout.
//
// name identifica il server nei log ("api", "metrics").
func ServeOnLifecycle(lc fx.Lifecycle, sh fx.Shutdowner, srv *http.Server, name string) {
	serveOnLifecycle(lc, sh, srv, name, func() (net.Listener, error) { return net.Listen("tcp", srv.Addr) })
}

// serveOnLifecycle è ServeOnLifecycle con l'apertura del listener iniettata: è ciò che permette di
// provare il ramo "il server muore a regime", che richiede di chiudere il listener da sotto.
func serveOnLifecycle(lc fx.Lifecycle, sh fx.Shutdowner, srv *http.Server, name string, listen func() (net.Listener, error)) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			ln, err := listen()
			if err != nil {
				return fmt.Errorf("%s: listen on %s: %w", name, srv.Addr, err)
			}
			log.Info().Str("server", name).Str("addr", ln.Addr().String()).Msg("HTTP server listening")
			go func() {
				if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error().Err(err).Str("server", name).Str("addr", srv.Addr).Msg("HTTP server terminato con errore, arresto dell'applicazione")
					if shErr := sh.Shutdown(fx.ExitCode(1)); shErr != nil {
						log.Error().Err(shErr).Str("server", name).Msg("shutdown dell'applicazione fallito")
					}
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info().Str("server", name).Msg("HTTP server shutting down")
			return srv.Shutdown(ctx)
		},
	})
}
