package core

import (
	"fmt"
	"net/http"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Field struct {
	Key   string
	Value any
}

func F(k string, v any) Field {
	return Field{Key: k, Value: v}
}

func (e *ApplicationError) Log(op fmt.Stringer, fields ...Field) {
	var ev *zerolog.Event
	if e.StatusCode == http.StatusNotFound {
		ev = log.Warn().Err(e)
	} else {
		ev = log.Error().Err(e)
	}
	ev = ev.Str("op", op.String())
	for _, f := range fields {
		ev = ev.Interface(f.Key, f.Value)
	}
	ev.Send()
}

type ApplicationError struct {
	StatusCode int    `json:"-" bson:"statusCode"`
	Ambit      string `json:"ambit"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (m *ApplicationError) Error() string {
	return m.Message
}
func (m *ApplicationError) GetStatus() int {
	return m.StatusCode
}

func TechnicalErrorWithError(err error) *ApplicationError {

	return &ApplicationError{
		StatusCode: 500,
		Message:    err.Error(),
		Ambit:      AppName,
		Code:       "TECH500",
	}
}

func (m *ApplicationError) IsTechnicalError() bool {

	return m.StatusCode == 500
}

func (m *ApplicationError) IsBusinessError() bool {

	return m.StatusCode == 422
}

func TechnicalErrorWithCodeAndMessage(code, message string) *ApplicationError {

	return &ApplicationError{
		StatusCode: 500,
		Message:    message,
		Ambit:      AppName,
		Code:       code,
	}
}

func NotFoundError() *ApplicationError {
	return &ApplicationError{
		StatusCode: 404,
		Ambit:      AppName,
		Code:       "NOT-FOUND",
		Message:    "Oggetto non trovato",
	}
}

func BusinessErrorWithError(err error) *ApplicationError {

	return &ApplicationError{
		StatusCode: 422,
		Message:    err.Error(),
		Ambit:      AppName,
		Code:       "BUS422",
	}
}

func BusinessErrorWithCodeAndMessage(code, message string) *ApplicationError {

	return &ApplicationError{
		StatusCode: 422,
		Message:    message,
		Ambit:      AppName,
		Code:       code,
	}
}
