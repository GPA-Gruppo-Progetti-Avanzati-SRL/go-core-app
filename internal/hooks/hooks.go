// Package hooks tiene le poche cose che vivono nella radice core ma servono a un package che core
// importa — observability (identità dell'app, sezione `metrics:`) e properties (il Validator).
// Quei package non possono importare core senza un ciclo, e core glieli passa da qui, nel proprio
// init e in ReadConfig. Essendo internal, un'app non può sostituirli: prima erano setter esportati
// (observability.SetIdentity, SetMetricsConfig, properties.SetValidator) con un commento che
// chiedeva di non usarli.
package hooks

import (
	"time"

	"github.com/go-playground/validator/v10"
)

// Identity è l'identità del processo nelle risorse OTel di metriche e tracce.
type Identity struct {
	Name    string // service.name: core.AppName
	Version string // service.version: core.BuildVersion
}

// IdentitySource la installa l'init di core. È una funzione e non un valore perché molte app
// assegnano core.AppName a mano dopo l'init, e l'identità deve seguire quell'assegnazione.
var IdentitySource = func() Identity { return Identity{} }

// ValidatorSource dà il validator dei tag `validate:` dei campi `prop:`: l'init di core installa
// core.Validator, così una RegisterValidation fatta dall'app vale anche lì. nil fuori da core.
var ValidatorSource func() *validator.Validate

// MetricsConfig è la sezione `metrics:` (esposta come observability.MetricsConfig).
type MetricsConfig struct {
	Host              string        `yaml:"host" mapstructure:"host" json:"host"`
	Port              int           `yaml:"port" mapstructure:"port" json:"port"`
	Pprof             bool          `yaml:"pprof" mapstructure:"pprof" json:"pprof"`
	ReadHeaderTimeout time.Duration `yaml:"read-header-timeout" mapstructure:"read-header-timeout" json:"read-header-timeout"`
}

// Metrics è la sezione `metrics:` letta da core.ReadConfig.
var Metrics MetricsConfig
