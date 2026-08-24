package core

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cast"
)

// Tag riconosciuti sui campi di una struct fornita a ProvideStruct per la CONFIGURAZIONE.
// Per i tag delle DIPENDENZE vedi modules_synth.go (InjectTag/FromTag/OptionalTag).
const (
	// PropTag marca un campo come property applicativa; il valore è la chiave dentro le Properties
	// (vuoto = nome del campo in minuscolo). SOLO i campi che lo portano sono toccati dal mapping.
	PropTag = "prop"
	// DefaultTag è il valore usato quando la chiave è assente dalle properties. È una stringa e passa
	// per la stessa conversione dei valori YAML (quindi "5s", "100", "true", "a,b" funzionano).
	DefaultTag = "default"
	// ValidateTag è il vincolo go-playground/validator applicato al singolo campo dopo il decode.
	ValidateTag = "validate"
)

// Properties è un blocco di configurazione applicativa (il `properties:` di un consumer Kafka o di un
// task batch). I valori conservano il tipo YAML nativo (stringa, intero, booleano, lista, mappa
// annidata): il modo raccomandato per leggerli è il mapping sui campi della struct via tag `prop:`
// (vedi BindProps); i getter qui sotto restano per le properties dinamiche, non strutturate.
//
// ATTENZIONE ALLE MAIUSCOLE: ReadConfig carica la config con viper, che abbassa ricorsivamente le
// chiavi (`workType:` nello YAML arriva qui come `worktype`). Per questo tutte le letture — getter,
// BindProps, warnUnclaimed — usano un confronto case-insensitive.
type Properties map[string]any

// lookup ritorna il valore della chiave con confronto case-insensitive (vedi nota sopra su viper).
func (p Properties) lookup(key string) (any, bool) {
	if v, ok := p[key]; ok {
		return v, true
	}
	for k, v := range p {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

// Has indica se la chiave è presente.
func (p Properties) Has(key string) bool { _, ok := p.lookup(key); return ok }

// GetString ritorna il valore o def se assente/non convertibile.
func (p Properties) GetString(key, def string) string {
	if v, ok := p.lookup(key); ok {
		if s, err := cast.ToStringE(v); err == nil {
			return s
		}
	}
	return def
}

// GetInt ritorna il valore intero o def se assente/non convertibile.
func (p Properties) GetInt(key string, def int) int {
	if v, ok := p.lookup(key); ok {
		if n, err := cast.ToIntE(v); err == nil {
			return n
		}
	}
	return def
}

// GetBool ritorna il valore booleano o def se assente/non convertibile.
func (p Properties) GetBool(key string, def bool) bool {
	if v, ok := p.lookup(key); ok {
		if b, err := cast.ToBoolE(v); err == nil {
			return b
		}
	}
	return def
}

// GetDuration ritorna la durata (es. "5s") o def se assente/non convertibile.
func (p Properties) GetDuration(key string, def time.Duration) time.Duration {
	if v, ok := p.lookup(key); ok {
		if d, err := cast.ToDurationE(v); err == nil {
			return d
		}
	}
	return def
}

// BindProps mappa props sui campi di target (un puntatore a struct) taggati `prop:`. È chiamata dal
// costruttore sintetizzato da ProvideStruct, quindi un errore fa fallire la costruzione del grafo fx:
// l'app non parte.
//
//	type importRunner struct {
//	    Data       IData         `inject:""`                          // iniettato da fx
//	    Collection string        `prop:"collection" validate:"required"`
//	    BatchLimit int           `prop:"batch-limit" default:"100"`
//	    Timeout    time.Duration `prop:"timeout" default:"5s"`
//	}
//
// I campi `prop:` non portano tag DI: dig non li vede mai, perché il costruttore fornito a fx è
// sintetizzato con un param object che contiene le sole dipendenze. Vengono comunque AZZERATI prima
// del decode, così nemmeno per vie indirette una property può ereditare un valore dal grafo fx.
//
// props nil è equivalente a props vuote: i `default:` vengono applicati e la validazione eseguita
// (quindi un `validate:"required"` senza valore fa fallire l'avvio, invece di restare a zero).
//
// Le chiavi presenti nelle properties e non reclamate da nessun campo sono ignorate e loggate a Warn
// (rete di sicurezza sui typo, senza bloccare l'avvio).
func BindProps(target any, props Properties) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("BindProps richiede un puntatore a struct, ricevuto %T", target)
	}
	elem := rv.Elem()
	t := elem.Type()

	// Input del decode: SOLO le chiavi dei campi taggati `prop:`, così i campi non taggati (dipendenze
	// fx, campi di lavorazione) non sono raggiungibili da mapstructure.
	in := make(map[string]any)
	claimed := make(map[string]bool)
	type propField struct {
		field reflect.StructField
		key   string
	}
	var fields []propField

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup(PropTag)
		if !ok {
			continue
		}
		if f.PkgPath != "" {
			return fmt.Errorf("campo %s.%s: il tag %q richiede un campo esportato", t.Name(), f.Name, PropTag)
		}
		key := tag
		if key == "" {
			key = strings.ToLower(f.Name)
		}
		claimed[strings.ToLower(key)] = true
		fields = append(fields, propField{field: f, key: key})

		// Azzera: un valore iniettato da fx non deve sopravvivere come property.
		fv := elem.Field(i)
		fv.Set(reflect.Zero(f.Type))

		if v, present := props.lookup(key); present {
			in[key] = v
		} else if def, hasDef := f.Tag.Lookup(DefaultTag); hasDef {
			in[key] = def
		}
	}

	// Nessun campo `prop:`: niente da mappare e nessun warning da dare (le chiavi non sono "non
	// reclamate", semplicemente non si usa questo canale).
	if len(fields) == 0 {
		return nil
	}

	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           target,
		TagName:          PropTag,
		WeaklyTypedInput: true, // le sostituzioni ${ENV_VAR} e i default sono stringhe
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	})
	if err != nil {
		return fmt.Errorf("properties: %w", err)
	}
	if err := dec.Decode(in); err != nil {
		return fmt.Errorf("properties: %w", err)
	}

	warnUnclaimed(t, props, claimed)

	// Validazione per campo (non sull'intera struct): evita di entrare nelle dipendenze iniettate, che
	// possono avere `validate:` propri, e permette di nominare la property nell'errore.
	for _, pf := range fields {
		rule, ok := pf.field.Tag.Lookup(ValidateTag)
		if !ok || rule == "" {
			continue
		}
		if err := Validator.Var(elem.FieldByIndex(pf.field.Index).Interface(), rule); err != nil {
			return fmt.Errorf("property %q (campo %s): %w", pf.key, pf.field.Name, err)
		}
	}

	return nil
}

// warnUnclaimed logga le chiavi delle properties che nessun campo `prop:` ha reclamato: tipicamente un
// typo in config. Non è un errore (le chiavi extra restano lecite, es. properties lette a runtime).
func warnUnclaimed(t reflect.Type, props Properties, claimed map[string]bool) {
	var extra []string
	for k := range props {
		if !claimed[strings.ToLower(k)] {
			extra = append(extra, k)
		}
	}
	if len(extra) == 0 {
		return
	}
	slices.Sort(extra)
	log.Warn().Str("type", t.Name()).Strs("keys", extra).
		Msg("properties non mappate su alcun campo `prop:` (typo in config?)")
}
