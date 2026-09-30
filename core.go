// Package core è il runtime delle applicazioni GPA: il boot (Boot, ReadConfig, Run), il registry
// fx (Provide*, Supply, Invoke, Module, ModuleClosed, Private, ProvideStruct), il MODE del processo
// e la sua identità, l'errore applicativo (Error, i costruttori base, AmbitErrors) e la validazione
// (ValidateStruct, IValidate). Il resto della libreria sta in un package per dominio — properties,
// observability, httpx, cli, utils, page.
package core

import (
	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app/internal/hooks"
	"github.com/go-playground/validator/v10"
)

// Identità del processo. BuildVersion, SHA e BuildDate li inietta il linker
// (-X github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app.BuildVersion=...), e se mancano Boot
// li riempie da debug.ReadBuildInfo; AppName e Logo li imposta Boot da App. Stanno nella radice
// perché è qui che i -X li cercano: spostarli romperebbe gli script di build di ogni app.
var (
	BuildVersion string
	BuildDate    string
	SHA          string
	AppName      string
	Logo         []byte
)

// Due package importati dalla radice hanno bisogno di qualcosa che vive qui, e non possono importare
// core senza un ciclo: observability l'identità (service.name e service.version di metriche e tracce),
// properties il Validator (i tag validate: dei campi prop:, con le RegisterValidation fatte
// dall'app). Glieli si passa da internal/hooks — internal perché un'app non li possa sostituire —
// come sorgenti lette a ogni uso, non copie: molte app assegnano ancora core.AppName a mano dopo
// l'init.
func init() {
	hooks.IdentitySource = func() hooks.Identity { return hooks.Identity{Name: AppName, Version: BuildVersion} }
	hooks.ValidatorSource = func() *validator.Validate { return Validator }
}
