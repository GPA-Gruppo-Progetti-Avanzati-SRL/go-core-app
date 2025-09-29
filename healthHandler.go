package core

import (
	"net/http"
)

// HealthHandler is a simple liveness endpoint that always returns HTTP 200 OK.
var HealthHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
})
