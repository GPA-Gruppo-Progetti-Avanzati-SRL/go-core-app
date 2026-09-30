package utils

import (
	"os"
	"sync"

	"github.com/rs/zerolog/log"
)

// UnknownHostname è ciò che GetHostname ritorna se il sistema operativo non dà un hostname.
const UnknownHostname = "unknown"

// GetHostname ritorna l'hostname del processo, letto una volta sola: non cambia durante la vita
// del processo, e chi lo scrive su ogni riga persistita (task_logs, work_items) lo chiamava a ogni
// scrittura — con un Warn per chiamata se falliva. Se os.Hostname fallisce o è vuoto ritorna
// UnknownHostname invece della stringa vuota: un campo "chi l'ha fatto" vuoto è indistinguibile da
// "non scritto", e prima go-core-batch aveva un secondo helper proprio per questa ragione, che
// divergeva da questo in caso di errore.
func GetHostname() string { return hostname() }

var hostname = sync.OnceValue(func() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		log.Warn().Err(err).Msgf("could not get hostname, using %q", UnknownHostname)
		return UnknownHostname
	}
	return h
})
