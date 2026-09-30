package core

import (
	"fmt"
	"reflect"
)

// TaggedField è un campo di una struct filtro che porta entrambi i tag chiesti a TaggedFields.
type TaggedField struct {
	Name  string // nome del campo Go, per i messaggi d'errore
	Key   string // valore del tag chiave (il campo mongo, la colonna SQL)
	Op    string // valore del tag operatore
	Value any
}

// TaggedFields percorre una struct filtro — o un puntatore a una struct — e ritorna, in ordine di
// dichiarazione, i campi che hanno sia keyTag sia opTag. Un campo senza uno dei due è ignorato; un
// campo col tag `omitempty` e al suo zero value è ignorato.
//
// È lo scheletro comune del filter builder di go-core-mongo (`field:`/`operator:`) e di quello di
// go-core-sql (`col:`/`op:`), che differiscono solo per i nomi dei tag e per ciò che producono.
// Averlo in un punto solo corregge insieme i due difetti che le copie condividevano:
//
//   - il valore si legge SOLO dopo aver verificato i tag: go-core-mongo lo leggeva prima, quindi
//     un qualsiasi campo non esportato — anche senza tag — faceva panicare reflect;
//   - un campo non esportato CON i due tag è un errore, non un panic: è un filtro scritto male,
//     e il valore non è leggibile.
func TaggedFields(v any, keyTag, opTag string) ([]TaggedField, error) {
	if v == nil {
		return nil, fmt.Errorf("filter cannot be nil")
	}
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil, fmt.Errorf("filter cannot be a nil pointer")
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("filter must be a struct, got %s", val.Kind())
	}

	typ := val.Type()
	var out []TaggedField
	for i := range typ.NumField() {
		f := typ.Field(i)
		key, op := f.Tag.Get(keyTag), f.Tag.Get(opTag)
		if key == "" || op == "" {
			continue
		}
		if !f.IsExported() {
			return nil, fmt.Errorf("filter field %s is tagged %s:%q but not exported: its value cannot be read", f.Name, keyTag, key)
		}
		fv := val.Field(i)
		if _, omit := f.Tag.Lookup("omitempty"); omit && fv.IsZero() {
			continue
		}
		out = append(out, TaggedField{Name: f.Name, Key: key, Op: op, Value: fv.Interface()})
	}
	return out, nil
}
