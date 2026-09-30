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

func (e *Error) Log(op fmt.Stringer, fields ...Field) {
	var ev *zerolog.Event
	if e.StatusCode == http.StatusNotFound {
		ev = log.Warn().Err(e)
	} else {
		ev = log.Error().Err(e)
	}
	if op != nil {
		ev = ev.Str("op", op.String())
	}
	// La causa non è nel testo di Error() (che resta il solo Message): senza questo
	// campo l'errore originale non comparirebbe da nessuna parte nei log. Si stampa
	// solo la causa reale — quella sintetica di Unwrap ripeterebbe Code e Message.
	if e.cause != nil {
		ev = ev.AnErr("cause", e.cause)
	}
	for _, f := range fields {
		ev = ev.Interface(f.Key, f.Value)
	}
	ev.Send()
}

// codeError è la foglia sintetica che Unwrap sintetizza quando non è stata allegata
// una causa reale: senza di essa un Error costruito senza errore avrebbe
// Unwrap() nil e la catena si fermerebbe, mentre uno costruito da un errore no.
//
// È un tipo dedicato e non un errors.New(message) per due motivi: resta distinguibile
// da una causa reale (Log non la stampa, perché ripeterebbe la riga), e il testo porta
// anche il Code. È sintetizzata al volo e non memorizzata, così riflette sempre Code e
// Message correnti anche dopo WithCode/WithMessage.
type codeError struct {
	code    string
	message string
}

func (c *codeError) Error() string { return c.code + ": " + c.message }

type Error struct {
	StatusCode int    `json:"-" bson:"statusCode"`
	Ambit      string `json:"ambit"`
	Code       string `json:"code"`
	Message    string `json:"message"`

	// cause è l'errore che ha originato questo Error. È NON esportato e
	// NON serializzato — json e bson ignorano i campi non esportati — quindi il body
	// della risposta HTTP e il documento persistito restano identici a prima.
	// Serve solo a rendere attraversabile la catena con errors.Is/errors.As.
	cause error
}

// Error ritorna il Message. Se il Message è vuoto (TechnicalError().WithCode(c), senza causa né
// messaggio) ritorna il Code: una stringa vuota in un log o in un %v non diceva nemmeno che
// c'era stato un errore.
func (m *Error) Error() string {
	if m.Message == "" {
		return m.Code
	}
	return m.Message
}
func (m *Error) GetStatus() int {
	return m.StatusCode
}

// Unwrap espone la causa a errors.Is/errors.As.
//
// Senza di essa la catena si interrompe su Error: TechnicalErrorWithError
// conserva solo err.Error() e il chiamante non può più interrogare l'errore originale
// (mongo.ErrNoDocuments, un *pgconn.PgError, uno store.RetryError...). Il compilatore
// accetta comunque errors.Is/errors.As su quella catena, quindi il ramo che ne dipende
// non fallisce: semplicemente non viene mai preso.
//
//	appErr := core.TechnicalError().WithCause(err)
//	errors.Is(appErr, mongo.ErrNoDocuments)   // ora attraversa fino a err
//
// Non ritorna mai nil: se non è stata allegata una causa reale con WithCause/WithError,
// sintetizza una foglia dal Code e dal Message correnti (vedi codeError), così la catena
// è completa comunque sia stato costruito l'errore.
func (m *Error) Unwrap() error {
	if m.cause != nil {
		return m.cause
	}
	return &codeError{code: m.Code, message: m.Message}
}

// I modificatori sotto sono ortogonali — uno per campo — e si compongono a partire da
// un costruttore base (TechnicalError, BusinessError, NotFoundError):
//
//	core.TechnicalError().WithCode("MON-AGGINC").WithMessage("aggiornamento incoerente").WithCause(err)
//	core.BusinessError().WithCause(err)
//
// Modificano il ricevente e lo ritornano (non ne fanno una copia): vanno usati sul
// valore appena costruito, come negli esempi, non su un errore condiviso.

// WithCode sostituisce il codice applicativo.
func (m *Error) WithCode(code string) *Error {
	m.Code = code
	return m
}

// WithMessage sostituisce il messaggio, che è ciò che Error() ritorna e ciò che finisce
// nel body della risposta HTTP.
func (m *Error) WithMessage(message string) *Error {
	m.Message = message
	return m
}

// WithAmbit sostituisce l'ambito, che i costruttori base riempiono con AppName.
func (m *Error) WithAmbit(ambit string) *Error {
	m.Ambit = ambit
	return m
}

// WithCause allega l'errore che ha originato questo Error, rendendolo
// raggiungibile da errors.Is/errors.As.
//
// Se il messaggio non è ancora stato impostato lo riempie con err.Error(): passare
// l'errore rende quindi WithMessage superfluo nel caso comune, in cui il testo da
// esporre È quello dell'errore.
//
//	core.TechnicalError().WithCause(err)                        // Message = err.Error()
//	core.TechnicalError().WithCode("MON-AGGINC").
//		WithMessage("aggiornamento incoerente").WithCause(err)   // Message resta il nostro
//
// Riempie solo un messaggio vuoto, non lo sovrascrive mai: l'ordine fra WithMessage e
// WithCause è quindi indifferente, e un messaggio esplicito vince sempre.
func (m *Error) WithCause(err error) *Error {
	m.cause = err
	if m.Message == "" && err != nil {
		m.Message = err.Error()
	}
	return m
}

// Ambit è la libreria di origine, da mettere con WithAmbit sugli errori nati DENTRO una
// libreria go-core: i costruttori base riempiono Ambit con AppName — cioè con l'app che
// riceve l'errore — quindi senza sovrascriverlo un guasto della libreria si presenta come
// un errore dell'applicazione, e chi legge il log non sa dove guardare.
const Ambit = "go-core-app"

// TechnicalError costruisce un errore tecnico (HTTP 500) con codice di default TECH500.
// Si compone con i modificatori With*:
//
//	core.TechnicalError().WithCause(err)
//	core.TechnicalError().WithCode("SEQ-INV").WithMessage("sequence is not an integer")
func TechnicalError() *Error {
	return &Error{
		StatusCode: 500,
		Ambit:      AppName,
		Code:       "TECH500",
	}
}

// BusinessError costruisce un errore applicativo (HTTP 422) con codice di default BUS422.
func BusinessError() *Error {
	return &Error{
		StatusCode: 422,
		Ambit:      AppName,
		Code:       "BUS422",
	}
}

// NotFoundError costruisce un 404 con codice e messaggio di default. Chi ha in mano
// l'errore del driver ce lo allega, così il chiamante può ancora distinguere un
// "nessun documento" da un 404 sintetico:
//
//	core.NotFoundError().WithCause(mongo.ErrNoDocuments)
func NotFoundError() *Error {
	return &Error{
		StatusCode: 404,
		Ambit:      AppName,
		Code:       "NOT-FOUND",
		Message:    "Oggetto non trovato",
	}
}

func (m *Error) IsTechnicalError() bool {
	return m.StatusCode == 500
}

func (m *Error) IsBusinessError() bool {
	return m.StatusCode == 422
}

// AmbitErrors costruisce gli Error di una libreria con il suo ambito già impostato. Ogni
// modulo go-core ne dichiara uno solo:
//
//	var errs = core.AmbitErrors{Ambit: Ambit}
//
//	return errs.Tech(CodeAcquire).WithCause(err)
//	return errs.NotFound().WithCause(mongo.ErrNoDocuments)
//
// Serve perché l'ambito è l'unico campo che si può dimenticare senza che nulla lo segnali: i
// costruttori base lo riempiono con AppName, quindi un errore nato in una libreria senza
// WithAmbit si presenta come un errore dell'applicazione che lo riceve. Prima ogni libreria
// aveva i suoi due helper scritti a mano (mongo, sql, batch) oppure la catena completa ripetuta
// a ogni sito (locker, auth, kafka).
//
// Ritorna un *Error nuovo a ogni chiamata, quindi i modificatori (WithMessage,
// WithCause, ...) si compongono come sui costruttori base.
type AmbitErrors struct {
	Ambit string
}

// Tech è TechnicalError() con l'ambito della libreria e il codice dato.
func (e AmbitErrors) Tech(code string) *Error {
	return TechnicalError().WithAmbit(e.Ambit).WithCode(code)
}

// Business è BusinessError() con l'ambito della libreria e il codice dato.
func (e AmbitErrors) Business(code string) *Error {
	return BusinessError().WithAmbit(e.Ambit).WithCode(code)
}

// NotFound è NotFoundError() con l'ambito della libreria: codice NOT-FOUND e messaggio di default.
func (e AmbitErrors) NotFound() *Error {
	return NotFoundError().WithAmbit(e.Ambit)
}
