package utils

import (
	"strconv"
	"time"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"

	"github.com/rs/zerolog/log"
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
		return time.Time{}, core.BusinessError().
			WithAmbit(core.Ambit).
			WithCode(ErrDateParse).
			WithMessage("StringToDate: data " + strconv.Quote(date) + " non conforme a " + core.DateFormat).
			WithCause(err)
	}

	return timestamp, nil
}

func StringToDatePtr(date string) *time.Time {
	if date == "" {
		return nil
	}
	timestamp, err := time.ParseInLocation(core.DateFormat, date, time.Local)
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
	timestamp, err := time.ParseInLocation(core.DateTimeFormat, date, time.Local)
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

	timestamp, err := time.ParseInLocation(core.DateTimeFormat, date, time.Local)
	if err != nil {
		log.Error().Msgf("StringToDateTimePtr Error parsing date: %s", err.Error())
		return nil
	}
	return &timestamp
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
