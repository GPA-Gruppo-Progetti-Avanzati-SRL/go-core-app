package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/go-playground/validator/v10"
)

type richiestaOrdine struct {
	Codice   string `validate:"required"`
	Quantita int    `validate:"gte=1"`
}

// configSezione ha i tag mapstructure di una sezione di config: è il caso in cui il nome Go
// e la chiave YAML differiscono.
type configSezione struct {
	DataType string `mapstructure:"dataType" validate:"required"`
	Soglia   int    `mapstructure:"soglia,omitempty" validate:"gte=1"`
	SenzaTag string `validate:"required"`
}

// TestValidateStruct_NominaLaChiaveDiConfigurazione: l'errore deve nominare la riga che chi
// scrive lo YAML ha davanti (`dataType`), non il campo Go (`DataType`) — altrimenti la
// corrispondenza fra errore e file la deve fare a mente.
func TestValidateStruct_NominaLaChiaveDiConfigurazione(t *testing.T) {
	appErr := ValidateStruct(configSezione{})
	if appErr == nil {
		t.Fatal("attese violazioni di validazione")
	}

	var verr validator.ValidationErrors
	if !errors.As(error(appErr), &verr) {
		t.Fatal("la ValidationErrors deve restare raggiungibile")
	}
	campi := map[string]bool{}
	for _, fe := range verr {
		campi[fe.Field()] = true
	}
	if !campi["dataType"] {
		t.Errorf("atteso il campo 'dataType' (tag mapstructure), ottenuti %v", campi)
	}
	// Le opzioni del tag non fanno parte del nome.
	if !campi["soglia"] {
		t.Errorf("atteso il campo 'soglia' senza le opzioni del tag, ottenuti %v", campi)
	}
	// Senza tag mapstructure — i DTO delle API — resta il nome Go.
	if !campi["SenzaTag"] {
		t.Errorf("senza tag mapstructure atteso il nome Go, ottenuti %v", campi)
	}
	if !strings.Contains(appErr.Message, "dataType") {
		t.Errorf("Message non nomina la chiave di configurazione: %q", appErr.Message)
	}
}

func TestValidateStruct_StructValida(t *testing.T) {
	if appErr := ValidateStruct(richiestaOrdine{Codice: "ABC", Quantita: 2}); appErr != nil {
		t.Fatalf("atteso nil, ottenuto %s", appErr.Message)
	}
}

func TestValidateStruct_ConservaLaValidationErrorsComeCausa(t *testing.T) {
	appErr := ValidateStruct(richiestaOrdine{Quantita: 0})
	if appErr == nil {
		t.Fatal("attesa una violazione di validazione")
	}
	if appErr.Code != ErrValidation {
		t.Errorf("Code = %q, atteso %q", appErr.Code, ErrValidation)
	}

	// Il beneficio: il chiamante recupera i campi falliti in modo programmatico,
	// invece di fare il parsing del messaggio già formattato.
	var verr validator.ValidationErrors
	if !errors.As(error(appErr), &verr) {
		t.Fatal("la ValidationErrors del validator deve restare raggiungibile con errors.As")
	}
	if len(verr) != 2 {
		t.Fatalf("attese 2 violazioni, ottenute %d", len(verr))
	}
	campi := map[string]string{}
	for _, fe := range verr {
		campi[fe.Field()] = fe.Tag()
	}
	if campi["Codice"] != "required" {
		t.Errorf("Codice: tag = %q, atteso required", campi["Codice"])
	}
	if campi["Quantita"] != "gte" {
		t.Errorf("Quantita: tag = %q, atteso gte", campi["Quantita"])
	}

	// Il messaggio resta quello di prima: la causa non lo contamina.
	if !strings.Contains(appErr.Message, "Codice") {
		t.Errorf("Message non nomina il campo fallito: %q", appErr.Message)
	}
}

func TestValidateStruct_ErroreNonDiValidazione(t *testing.T) {
	// Validator.Struct su un non-struct ritorna un *InvalidValidationError: non è una
	// ValidationErrors, ma deve comunque finire nella catena.
	appErr := ValidateStruct("non è una struct")
	if appErr == nil {
		t.Fatal("atteso un errore")
	}
	var ive *validator.InvalidValidationError
	if !errors.As(error(appErr), &ive) {
		t.Error("anche l'InvalidValidationError deve restare raggiungibile")
	}
}
