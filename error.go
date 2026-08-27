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
	// La causa non è nel testo di Error() (che resta il solo Message): senza questo
	// campo l'errore originale non comparirebbe da nessuna parte nei log. La causa
	// sintetica dei costruttori code+message è invece omessa: ripeterebbe Message.
	if _, sintetica := e.cause.(*codeError); e.cause != nil && !sintetica {
		ev = ev.AnErr("cause", e.cause)
	}
	for _, f := range fields {
		ev = ev.Interface(f.Key, f.Value)
	}
	ev.Send()
}

// codeError è la causa sintetica dei costruttori che ricevono code+message invece di
// un errore: senza di essa quegli ApplicationError avrebbero Unwrap() nil e la catena
// si fermerebbe, mentre quelli costruiti da un errore no. Con codeError la catena è
// sempre completa e termina su una foglia che porta il codice applicativo.
//
// È un tipo dedicato e non un errors.New(message) per due motivi: la foglia resta
// distinguibile da una causa reale (WithCause la sostituisce, e Log non la stampa
// perché non aggiunge nulla alla riga), e il testo porta anche il Code.
type codeError struct {
	code    string
	message string
}

func (c *codeError) Error() string { return c.code + ": " + c.message }

type ApplicationError struct {
	StatusCode int    `json:"-" bson:"statusCode"`
	Ambit      string `json:"ambit"`
	Code       string `json:"code"`
	Message    string `json:"message"`

	// cause è l'errore che ha originato questo ApplicationError. È NON esportato e
	// NON serializzato — json e bson ignorano i campi non esportati — quindi il body
	// della risposta HTTP e il documento persistito restano identici a prima.
	// Serve solo a rendere attraversabile la catena con errors.Is/errors.As.
	cause error
}

func (m *ApplicationError) Error() string {
	return m.Message
}
func (m *ApplicationError) GetStatus() int {
	return m.StatusCode
}

// Unwrap espone la causa a errors.Is/errors.As.
//
// Senza di essa la catena si interrompe su ApplicationError: TechnicalErrorWithError
// conserva solo err.Error() e il chiamante non può più interrogare l'errore originale
// (mongo.ErrNoDocuments, un *pgconn.PgError, uno store.RetryError...). Il compilatore
// accetta comunque errors.Is/errors.As su quella catena, quindi il ramo che ne dipende
// non fallisce: semplicemente non viene mai preso.
//
//	appErr := core.TechnicalErrorWithError(err)
//	errors.Is(appErr, mongo.ErrNoDocuments)   // ora attraversa fino a err
//
// Non ritorna mai nil per gli ApplicationError costruiti dai costruttori: quelli che
// ricevono un errore lo conservano, quelli che ricevono code+message hanno una causa
// sintetica (vedi codeError).
func (m *ApplicationError) Unwrap() error {
	return m.cause
}

// WithCause allega la causa a un ApplicationError già costruito ed è pensata per i
// costruttori *WithCodeAndMessage, che il codice specifico ce l'hanno ma l'errore
// originale lo perderebbero:
//
//	return core.TechnicalErrorWithCodeAndMessage("MON-AGGINC", "aggiornamento incoerente").WithCause(err)
//
// Sostituisce la causa sintetica che quei costruttori mettono di default: una causa
// reale vince sempre. Modifica il ricevente e lo ritorna (non ne fa una copia): va usata
// sul valore appena costruito, come nell'esempio, non su un errore condiviso.
func (m *ApplicationError) WithCause(err error) *ApplicationError {
	m.cause = err
	return m
}

func TechnicalErrorWithError(err error) *ApplicationError {

	return &ApplicationError{
		StatusCode: 500,
		Message:    err.Error(),
		Ambit:      AppName,
		Code:       "TECH500",
		cause:      err,
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
		cause:      &codeError{code: code, message: message},
	}
}

func NotFoundError() *ApplicationError {
	return &ApplicationError{
		StatusCode: 404,
		Ambit:      AppName,
		Code:       "NOT-FOUND",
		Message:    "Oggetto non trovato",
		cause:      &codeError{code: "NOT-FOUND", message: "Oggetto non trovato"},
	}
}

func BusinessErrorWithError(err error) *ApplicationError {

	return &ApplicationError{
		StatusCode: 422,
		Message:    err.Error(),
		Ambit:      AppName,
		Code:       "BUS422",
		cause:      err,
	}
}

func BusinessErrorWithCodeAndMessage(code, message string) *ApplicationError {

	return &ApplicationError{
		StatusCode: 422,
		Message:    message,
		Ambit:      AppName,
		Code:       code,
		cause:      &codeError{code: code, message: message},
	}
}
