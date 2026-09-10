package core

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/locales/it"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog/log"
)

const ErrValidation = "ERR_VALIDATION"

var Validator = validator.New()
var Translator = ut.New(it.New(), it.New())

// IValidate lo implementa la config che ha vincoli NON esprimibili con i tag del validator:
// riferimenti incrociati fra sezioni, regole condizionate dal tipo di un'entità referenziata,
// coerenza fra liste. Boot lo cerca sulle due sezioni della config con una type assertion
// (vedi boot.go), quindi è opzionale: le app che non ne hanno bisogno non cambiano.
//
// L'errore ritornato è già il messaggio per l'operatore: Boot lo stampa e ferma l'avvio.
// Con più violazioni conviene aggregarle con errors.Join, invece di far scoprire la seconda
// solo dopo aver corretto la prima.
type IValidate interface {
	Validate() error
}

func init() {
	// Il nome del campo negli errori di validazione è quello che chi scrive lo YAML ha davanti
	// (`dataType`) e non quello della struct Go (`DataType`): il tag `mapstructure` è la chiave
	// con cui viper ha popolato il campo, quindi l'errore nomina la riga di configurazione da
	// correggere. Le struct senza tag `mapstructure` — i DTO delle API — restano al nome Go.
	Validator.RegisterTagNameFunc(func(f reflect.StructField) string {
		name := strings.SplitN(f.Tag.Get("mapstructure"), ",", 2)[0]
		if name == "" || name == "-" {
			return f.Name
		}
		return name
	})
}

func ValidateStruct(i any) *ApplicationError {

	if verr := Validator.Struct(i); verr != nil {
		var errorMessages []string
		var errmsg string

		log.Debug().Err(verr).Msg("Validation error")

		if errValidate, ok := errors.AsType[validator.ValidationErrors](verr); ok {
			for _, everr := range errValidate {
				errorMessages = append(errorMessages, fmt.Sprintf("Field '%s': %s.", everr.Field(), everr.Translate(Translator.GetFallback())))
			}
			errmsg = fmt.Sprintf("Validation errors: %s", errorMessages)
		} else {
			errmsg = fmt.Sprintf("Validation error: %s", verr.Error())
		}

		// La causa è l'errore del validator: con WithCause il chiamante può recuperarlo
		// con errors.As e ispezionare i singoli campi falliti, invece di dover fare il
		// parsing del messaggio già formattato.
		return TechnicalError().WithAmbit(Ambit).WithCode(ErrValidation).WithMessage(errmsg).WithCause(verr)

	}
	return nil
}
