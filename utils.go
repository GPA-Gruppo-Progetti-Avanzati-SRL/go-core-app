package core

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// UnknownHostname è ciò che GetHostname ritorna se il sistema operativo non dà un hostname.
const UnknownHostname = "unknown"

// GetHostname ritorna l'hostname del processo, letto una volta sola: non cambia durante la vita
// del processo, e chi lo scrive su ogni riga persistita (task_logs, work_items) lo chiamava a ogni
// scrittura — con un Warn per chiamata se falliva. Se os.Hostname fallisce o è vuoto ritorna
// UnknownHostname invece della stringa vuota: un campo "chi l'ha fatto" vuoto è indistinguibile da
// "non scritto", e prima go-core-batch aveva un secondo helper proprio per questa ragione, che
// divergeva da questo in caso di errore.
func GetHostname() string { return hostname() }

var hostname = sync.OnceValue(func() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		log.Warn().Err(err).Msgf("could not get hostname, using %q", UnknownHostname)
		return UnknownHostname
	}
	return h
})

func GetTimestamp() string {
	date := time.Now()
	stringDate := date.Format("20060102150405")
	return stringDate
}

// ErrDateParse: stringa non conforme a DateFormat. Sostituisce il codice segnaposto "99999",
// che non diceva nulla a chi lo riceveva.
const ErrDateParse = "ERR-DATE"

func StringToDate(date string) (time.Time, *ApplicationError) {
	timestamp, err := time.ParseInLocation(DateFormat, date, time.Local)

	if err != nil {
		return time.Time{}, BusinessError().
			WithAmbit(Ambit).
			WithCode(ErrDateParse).
			WithMessage("StringToDate: data " + strconv.Quote(date) + " non conforme a " + DateFormat).
			WithCause(err)
	}

	return timestamp, nil
}

func StringToDatePtr(date string) *time.Time {
	if date == "" {
		return nil
	}
	timestamp, err := time.ParseInLocation(DateFormat, date, time.Local)
	if err != nil {
		log.Error().Msgf("StringToDatePtr Error parsing date: %s", err.Error())
		return nil
	}

	return &timestamp
}

func StringToDateTime(date string) time.Time {
	if date == "" {
		return time.Time{}
	}
	timestamp, err := time.ParseInLocation(DateTimeFormat, date, time.Local)
	if err != nil {
		log.Error().Msgf("StringToDateTime Error parsing date: %s", err.Error())
		return time.Time{}
	}
	return timestamp
}

func StringToDateTimePtr(date string) *time.Time {
	if date == "" {
		return nil
	}

	timestamp, err := time.ParseInLocation(DateTimeFormat, date, time.Local)
	if err != nil {
		log.Error().Msgf("StringToDateTimePtr Error parsing date: %s", err.Error())
		return nil
	}
	return &timestamp
}

func DateToString(date time.Time) string {
	return date.Format(DateFormat)
}

func StringPtrToString(s *string) string {
	if s == nil {
		return ""
	} else {
		return *s
	}

}

func DateTimeToString(date time.Time) string {
	return date.Format(DateTimeFormat)
}

func NowTime() time.Time {
	return time.Now()
}

func NowString() string {
	return NowTime().Format(DateTimeFormat)
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
	return new(date.Format(DateFormat))
}

func GetMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
