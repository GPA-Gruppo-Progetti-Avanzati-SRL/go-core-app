// Package utils raccoglie le utility senza un dominio proprio: cifratura del token (Encrypt,
// Decrypt), conversioni di date, concorrenza limitata (ConcurrentTwo, ConcurrentN), lo scheletro
// dei filter builder a tag (TaggedFields) e l'hostname del processo.
package utils

import (
	"strconv"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
)

func GetTimestamp() string {
	date := time.Now()
	stringDate := date.Format("20060102150405")
	return stringDate
}

// ErrDateParse: stringa non conforme a core.DateFormat. Sostituisce il codice segnaposto "99999",
// che non diceva nulla a chi lo riceveva.
const ErrDateParse = "ERR-DATE"

func StringToDate(date string) (time.Time, *core.Error) {
	timestamp, err := time.ParseInLocation(core.DateFormat, date, time.Local)

	if err != nil {
		return time.Time{}, parseErr("StringToDate", date, core.DateFormat, err)
	}

	return timestamp, nil
}

// parseErr è l'errore di una conversione rifiutata: lo stesso codice per le quattro funzioni, con
// la funzione e il formato atteso nel messaggio.
func parseErr(fn, value, layout string, err error) *core.Error {
	return core.BusinessError().
		WithAmbit(core.Ambit).
		WithCode(ErrDateParse).
		WithMessage(fn + ": data " + strconv.Quote(value) + " non conforme a " + layout).
		WithCause(err)
}

// StringToDatePtr è StringToDate con l'assenza: "" vale nil, senza errore. Una stringa non vuota e
// malformata è un errore e non un nil — prima ritornava nil anche lì, quindi una data sbagliata
// diventava una data assente e spariva senza che il chiamante potesse saperlo.
func StringToDatePtr(date string) (*time.Time, *core.Error) {
	if date == "" {
		return nil, nil
	}
	timestamp, err := time.ParseInLocation(core.DateFormat, date, time.Local)
	if err != nil {
		return nil, parseErr("StringToDatePtr", date, core.DateFormat, err)
	}
	return &timestamp, nil
}

// StringToDateTime converte una data con ora (core.DateTimeFormat); "" vale il tempo zero. Una
// stringa malformata è un errore: prima ritornava il tempo zero, indistinguibile da "".
func StringToDateTime(date string) (time.Time, *core.Error) {
	if date == "" {
		return time.Time{}, nil
	}
	timestamp, err := time.ParseInLocation(core.DateTimeFormat, date, time.Local)
	if err != nil {
		return time.Time{}, parseErr("StringToDateTime", date, core.DateTimeFormat, err)
	}
	return timestamp, nil
}

// StringToDateTimePtr è StringToDateTime con l'assenza: "" vale nil, una stringa malformata è un errore.
func StringToDateTimePtr(date string) (*time.Time, *core.Error) {
	if date == "" {
		return nil, nil
	}
	timestamp, err := time.ParseInLocation(core.DateTimeFormat, date, time.Local)
	if err != nil {
		return nil, parseErr("StringToDateTimePtr", date, core.DateTimeFormat, err)
	}
	return &timestamp, nil
}

func DateToString(date time.Time) string {
	return date.Format(core.DateFormat)
}

func StringPtrToString(s *string) string {
	if s == nil {
		return ""
	} else {
		return *s
	}

}

func DateTimeToString(date time.Time) string {
	return date.Format(core.DateTimeFormat)
}

func NowTime() time.Time {
	return time.Now()
}

func NowString() string {
	return NowTime().Format(core.DateTimeFormat)
}

func DateToStringPtr(date time.Time) *string {
	return dateToPtr(&date)
}

func DatePtrToStringPtr(date *time.Time) *string {
	return dateToPtr(date)
}

func DatePtrToString(date *time.Time) string {
	if date == nil {
		return ""
	}
	d := dateToPtr(date)
	if d == nil {
		return ""
	} else {
		return *d
	}
}

func dateToPtr(date *time.Time) *string {
	if date == nil {
		return nil
	}
	if date.IsZero() {
		return nil
	}
	return new(date.Format(core.DateFormat))
}

func GetMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
