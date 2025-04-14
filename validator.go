package core

import (
	"errors"
	"fmt"
	"github.com/go-playground/locales/it"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog/log"
)

const ErrValidation = "ERR_VALIDATION"

var Validator = validator.New()
var Tranlator = ut.New(it.New(), it.New())

func ValidateStruct(i interface{}) *ApplicationError {

	if verr := Validator.Struct(i); verr != nil {
		var errValidate validator.ValidationErrors
		var errorMessages []string
		var errmsg string

		log.Debug().Err(verr).Msg("Validation error")

		if errors.As(verr, &errValidate) {
			for _, everr := range errValidate {
				errorMessages = append(errorMessages, fmt.Sprintf("Field '%s': %s.", everr.Field(), everr.Translate(Tranlator.GetFallback())))
			}
			errmsg = fmt.Sprintf("Validation errors: %s", errorMessages)
		} else {
			errmsg = fmt.Sprintf("Validation error: %s", verr.Error())
		}

		return TechnicalErrorWithCodeAndMessage(ErrValidation, errmsg)

	}
	return nil
}
