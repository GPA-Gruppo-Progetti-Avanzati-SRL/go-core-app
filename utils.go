package core

import (
	"fmt"
	"github.com/rs/zerolog/log"
	"os"
	"strconv"
	"strings"
	"time"
)

func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Warn().Msgf("Could not get hostname: %s", err.Error())
	}
	return hostname
}

func GetTimestamp() string {
	date := time.Now()
	stringDate := date.Format("20060102150405")
	return stringDate
}

//MOTORE ^\\d{14}-[A-Z]{4}-[a-z0-9A-Z\\-]{30,50}$

func ConvertStringToTimeDate(input string) (time.Time, error) {
	// data stringa con formato "yyyy-mm-dd"
	// Separiamo anno, mese e giorno dalla stringa
	parts := strings.Split(input, "-")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("formato non valido: %s", input)
	}

	// Convertiamo i valori in interi
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, fmt.Errorf("anno non valido: %v", err)
	}

	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("mese non valido: %v", err)
	}

	day, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, fmt.Errorf("giorno non valido: %v", err)
	}

	// Creiamo l'oggetto time.Time con time.Date
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return date, nil
}

func StringToDate(date string) (time.Time, *ApplicationError) {
	timestamp, err := time.ParseInLocation(DateFormat, date, time.Local)

	if err != nil {
		return time.Time{}, &ApplicationError{
			StatusCode: 422,
			Ambit:      "Utils Methods - StringToDate",
			Code:       "99999",
			Message:    err.Error(),
		}
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
	str := date.Format(DateFormat)
	return &str
}

func GetMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
