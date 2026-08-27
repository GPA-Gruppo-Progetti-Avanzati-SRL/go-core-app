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
