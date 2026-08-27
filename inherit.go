package core

import (
	"fmt"
	"reflect"
)

// Inherit e IsZeroStruct implementano l'eredità fra due livelli di configurazione omologhi — il blocco
// globale e il blocco che lo sovrascrive — quando i due livelli sono lo STESSO tipo Go.
//
// È il pattern della config a due livelli: `server.consumer` e `processors[].consumer` in
// go-core-kafka, e ogni altro posto in cui un default condiviso viene raffinato per istanza. La regola
// è una sola per ogni campo — "non valorizzato ⇒ prendi il valore del livello superiore" — quindi
// scriverla campo per campo non aggiunge informazione: aggiunge un posto dove dimenticarla, e la
// dimenticanza è INVISIBILE. Un campo nuovo senza il suo ramo non eredita: prende lo zero invece del
// valore globale, nessun errore lo dice, e il sintomo è un valore sbagliato in esercizio.
//
// Il prezzo è che la regola per tipo va decisa qui una volta per tutte, ed è questo:
//
//	*T                  nil eredita — è anche il modo di sovrascrivere con un valore "spegnente"
//	                    (0, false, 1), che con un valore semplice sarebbe indistinguibile da "assente"
//	string              "" eredita
//	int/int64/Duration  <= 0 eredita
//	float               <= 0 eredita
//	map[K]V             FUSA chiave per chiave: il livello globale è la base, dst vince sui conflitti
//	                    (aggiungere una chiave in basso non fa perdere quelle comuni)
//	slice               nil o vuota eredita, altrimenti sostituisce (una lista non si fonde: l'ordine
//	                    e la completezza sono del livello che la scrive)
//	bool                MAI: false è indistinguibile da "non scritto" — chi ha bisogno di ereditare
//	                    un booleano usa *bool
//	struct              ricorsione
//
// Un campo di un tipo non previsto fa PANICARE: è un errore di programmazione da correggere al primo
// test, e il silenzio alternativo sarebbe un campo che non eredita senza che nulla lo segnali.

// Inherit riempie i campi non valorizzati di dst con i corrispondenti di src, secondo la tabella
// sopra. dst e src sono lo stesso tipo per costruzione, quindi non esiste il caso "blocchi
// disallineati". Solo i campi esportati sono considerati.
//
//	func (t ConsumerTuning) inherit(g ConsumerTuning) ConsumerTuning {
//	    core.Inherit(&t, &g)
//	    return t
//	}
func Inherit[T any](dst, src *T) {
	d := reflect.ValueOf(dst).Elem()
	if d.Kind() != reflect.Struct {
		panic(fmt.Sprintf("core.Inherit: %s non è una struct", d.Type()))
	}
	inheritValue(d, reflect.ValueOf(src).Elem(), d.Type().Name())
}

// IsZeroStruct dice se ogni campo esportato di v è al suo valore "non valorizzato", con le stesse
// regole di Inherit. Serve a distinguere "questo blocco non è stato scritto" da "è stato scritto con
// valori che coincidono col default" — una differenza che di solito vale un avviso al boot, e che
// ri-elencando i campi a mano si perde appena se ne aggiunge uno.
//
// Nota: una mappa vuota ma non nil conta come non valorizzata (a differenza di reflect.Value.IsZero).
func IsZeroStruct[T any](v T) bool {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Struct {
		panic(fmt.Sprintf("core.IsZeroStruct: %s non è una struct", rv.Type()))
	}
	return isZeroValue(rv, rv.Type().Name())
}

func inheritValue(dst, src reflect.Value, owner string) {
	t := dst.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		dv, sv := dst.Field(i), src.Field(i)
		where := owner + "." + f.Name

		switch dv.Kind() {
		case reflect.Pointer, reflect.Interface:
			if dv.IsNil() {
				dv.Set(sv)
			}
		case reflect.String:
			if dv.Len() == 0 {
				dv.Set(sv)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if dv.Int() <= 0 {
				dv.Set(sv)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if dv.Uint() == 0 {
				dv.Set(sv)
			}
		case reflect.Float32, reflect.Float64:
			if dv.Float() <= 0 {
				dv.Set(sv)
			}
		case reflect.Map:
			dv.Set(mergeMaps(dv, sv))
		case reflect.Slice:
			if dv.Len() == 0 {
				dv.Set(sv)
			}
		case reflect.Bool:
			// Nessuna eredità possibile: false è indistinguibile da "non scritto".
		case reflect.Struct:
			inheritValue(dv, sv, where)
		default:
			panic(fmt.Sprintf("core.Inherit: %s è di tipo %s, non gestito: aggiungere il caso in inheritValue (lasciarlo passare significherebbe un campo che non eredita, in silenzio)", where, dv.Kind()))
		}
	}
}

// mergeMaps ritorna una mappa NUOVA con le chiavi di base (il livello globale) sovrascritte da quelle
// di over. Nuova e non modificata in place: il blocco globale è condiviso da tutti i livelli che ne
// discendono, e mutarlo farebbe apparire in uno le chiavi aggiunte da un altro.
func mergeMaps(over, base reflect.Value) reflect.Value {
	if over.Len() == 0 && base.Len() == 0 {
		return reflect.Zero(over.Type()) // nil, non una mappa vuota: "non valorizzata" resta tale
	}
	out := reflect.MakeMapWithSize(over.Type(), over.Len()+base.Len())
	// base prima, over dopo: sulle chiavi comuni vince il livello più specifico.
	for _, m := range []reflect.Value{base, over} {
		for iter := m.MapRange(); iter.Next(); {
			out.SetMapIndex(iter.Key(), iter.Value())
		}
	}
	return out
}

func isZeroValue(v reflect.Value, owner string) bool {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fv := v.Field(i)
		where := owner + "." + f.Name

		switch fv.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !fv.IsNil() {
				return false
			}
		case reflect.String, reflect.Map, reflect.Slice:
			if fv.Len() != 0 {
				return false
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if fv.Int() != 0 {
				return false
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if fv.Uint() != 0 {
				return false
			}
		case reflect.Float32, reflect.Float64:
			if fv.Float() != 0 {
				return false
			}
		case reflect.Bool:
			if fv.Bool() {
				return false
			}
		case reflect.Struct:
			if !isZeroValue(fv, where) {
				return false
			}
		default:
			panic(fmt.Sprintf("core.IsZeroStruct: %s è di tipo %s, non gestito", where, fv.Kind()))
		}
	}
	return true
}
