package core

import (
	"fmt"
	"reflect"
	"strings"
)

// Tag riconosciuti sui campi DIPENDENZA di una struct fornita a ProvideStruct. Sono tag GPA: il
// synthor li traduce nei tag che dig si aspetta, così la struct dell'app non deve conoscere il
// vocabolario di dig. Per i campi di configurazione vedi props.go (PropTag/DefaultTag/ValidateTag).
//
//	`inject:""`             → dipendenza semplice
//	`inject:"primary"`      → name:"primary"
//	`from:"import_hooks"`   → group:"import_hooks"
//	`optional:"true"`       → optional:"true" (invariato)
const (
	InjectTag   = "inject"
	FromTag     = "from"
	OptionalTag = "optional"
)

var (
	inType    = reflect.TypeFor[In]()
	outType   = reflect.TypeFor[Out]()
	errorType = reflect.TypeFor[error]()
)

// ProvideStruct fornisce a fx un costruttore SINTETIZZATO per il tipo struct T, con questo contratto
// sui campi di T:
//
//	`inject:` / `from:` / `optional:`  → dipendenza: dig la vede (tradotta nei suoi tag)
//	`prop:`                            → property: invisibile a dig, riempita da BindProps
//	nessun tag                         → campo di lavorazione: ignorato, resta al valore zero
//
// La struct NON deve embeddare core.In (è un errore al wiring): il marker lo porta il param object
// sintetico, e accettarlo lascerebbe passare struct scritte per la vecchia semantica, con le
// dipendenze non taggate silenziosamente a nil.
//
//	type importRunner struct {
//	    Data   IData   `inject:"primary"`
//	    Hooks  []IHook `from:"import_hooks"`
//	    Folder string  `prop:"folder" validate:"required"`
//	    buf    []byte  // lavorazione
//	}
//
// mk riceve il *T già popolato (dipendenze iniettate + properties mappate) e ritorna il valore da
// registrare: è l'unico punto che conosce staticamente T ed R.
//
//	owner  etichetta usata negli errori (es. `corekafka: consumer "condizione"`)
//	props  properties da bindare sui campi `prop:` (nil = solo default e validazione)
//	group  value group fx in cui registrare il risultato; "" = provide semplice
//
// Una struct che non si riesce a rappresentare è un errore di programmazione, non di configurazione:
// panic subito, al wiring.
func ProvideStruct[T any, R any](mk func(*T) R, owner string, props Properties, group string, modes ...string) {
	if !IsMode(modes...) {
		return
	}
	ctor, err := synthCtor(reflect.TypeFor[T](), reflect.TypeFor[R](), group, owner, props,
		func(ptr any) any { return mk(ptr.(*T)) })
	if err != nil {
		panic(fmt.Sprintf("%s: %v", owner, err))
	}
	Provide(ctor, modes...)
}

// dep è una dipendenza di T riconosciuta dal synthor: idx è l'indice del campo in T, tag è quello
// tradotto per dig, checkable indica che la verifichiamo noi per dare un errore contestualizzato.
type dep struct {
	idx       int
	tag       reflect.StructTag
	checkable bool
}

// synthCtor sintetizza il costruttore fx per il tipo struct t:
//
//	func(<param object con le SOLE dipendenze di t>) (<risultato, eventualmente nel value group>, error)
//
// Il punto è che dig non vede mai t: vede un param object sintetico che contiene solo i campi
// dipendenza. I campi property e quelli di lavorazione gli sono quindi invisibili e NON richiedono
// `optional:"true"` nell'app; le property le riempie BindProps dopo l'iniezione.
//
// Il costruttore è creato con reflect.MakeFunc, quindi fx non ha una location sensata da mostrare
// (dig.LocationForPC non è esposto da fx) e riporterebbe `reflect.makeFuncStub`. Per non perdere il
// contesto sugli errori più frequenti, le dipendenze **nil-abili** (puntatori, interfacce, mappe,
// slice, chan, func) sono dichiarate `optional:"true"` nel param object sintetico e verificate qui:
// così una dipendenza mancante produce un errore che nomina owner, campo e tipo. Le dipendenze non
// nil-abili (valori struct, param object annidati, interi) restano obbligatorie per dig e in quel caso
// il messaggio è quello di fx, col tipo mancante ma senza location utile.
func synthCtor(t, resultType reflect.Type, group, owner string, props Properties, mk func(ptr any) any) (ctor any, err error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("il tipo deve essere una struct, ricevuto %s", t)
	}
	// reflect.StructOf panica su input che non sa rappresentare: trasformiamo il panic in errore per
	// dare un messaggio contestualizzato al wiring.
	defer func() {
		if r := recover(); r != nil {
			ctor, err = nil, fmt.Errorf("impossibile sintetizzare il costruttore per %s: %v", t, r)
		}
	}()

	deps, err := collectDeps(t, owner)
	if err != nil {
		return nil, err
	}

	// Param object sintetico: marker In + i soli campi dipendenza, con i tag tradotti per dig.
	// Anonymous=false anche per i campi embeddati: a dig serve solo il tipo (un param object annidato
	// è riconosciuto dal tipo, non dall'embedding), e StructOf panica sugli embedded con metodi. Il
	// valore lo ricopiamo per indice.
	fields := []reflect.StructField{{Name: "In", Type: inType, Anonymous: true}}
	for _, d := range deps {
		f := t.Field(d.idx)
		fields = append(fields, reflect.StructField{Name: f.Name, Type: f.Type, Tag: d.tag})
	}
	paramType := reflect.StructOf(fields)

	// Risultato: dentro un value group serve un result object con il marker Out; senza group il
	// costruttore ritorna direttamente R.
	outStructType := resultType
	if group != "" {
		outStructType = reflect.StructOf([]reflect.StructField{
			{Name: "Out", Type: outType, Anonymous: true},
			{Name: "Registration", Type: resultType, Tag: reflect.StructTag(`group:"` + group + `"`)},
		})
	}

	fnType := reflect.FuncOf([]reflect.Type{paramType}, []reflect.Type{outStructType, errorType}, false)
	fn := reflect.MakeFunc(fnType, func(args []reflect.Value) []reflect.Value {
		zero := reflect.New(outStructType).Elem()
		fail := func(e error) []reflect.Value {
			return []reflect.Value{zero, reflect.ValueOf(&e).Elem()}
		}

		ptr := reflect.New(t) // *T
		for k, d := range deps {
			ptr.Elem().Field(d.idx).Set(args[0].Field(k + 1))
		}

		for _, d := range deps {
			if !d.checkable {
				continue
			}
			if ptr.Elem().Field(d.idx).IsZero() {
				f := t.Field(d.idx)
				return fail(fmt.Errorf("%s (%s): dipendenza mancante nel grafo fx: campo %s di tipo %s (manca un provider?)",
					owner, t, f.Name, f.Type))
			}
		}

		if bindErr := BindProps(ptr.Interface(), props); bindErr != nil {
			return fail(fmt.Errorf("%s: %w", owner, bindErr))
		}

		res := reflect.New(outStructType).Elem()
		v := reflect.ValueOf(mk(ptr.Interface()))
		if group != "" {
			res.Field(1).Set(v)
		} else {
			res.Set(v)
		}
		return []reflect.Value{res, reflect.Zero(errorType)}
	})

	return fn.Interface(), nil
}

// collectDeps applica il contratto sui campi di t e ritorna le sole dipendenze, con il tag già
// tradotto per dig.
func collectDeps(t reflect.Type, owner string) ([]dep, error) {
	// core.In è il marker di dig e qui non ha senso: il param object lo mette il synthor. Se lo
	// accettassimo, una struct scritta per la vecchia semantica (dipendenze non taggate) vedrebbe i
	// suoi campi trattati come stato interno e resterebbe con le dipendenze a nil, senza che nulla
	// fallisca. Meglio un errore che dice cosa fare.
	if embedsIn(t) {
		return nil, fmt.Errorf("la struct non deve embeddare core.In: le dipendenze si dichiarano col tag `inject:\"\"` (`inject:\"nome\"` per una dipendenza named, `from:\"gruppo\"` per un value group). core.In resta valido nei param object dei costruttori scritti a mano passati a core.Provide")
	}

	var deps []dep
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		_, isProp := f.Tag.Lookup(PropTag)
		injectVal, hasInject := f.Tag.Lookup(InjectTag)
		fromVal, hasFrom := f.Tag.Lookup(FromTag)
		isOptional := f.Tag.Get(OptionalTag) == "true"

		if isProp {
			if hasInject || hasFrom {
				return nil, fmt.Errorf("campo %s: il tag %q non può essere combinato con %q/%q",
					f.Name, PropTag, InjectTag, FromTag)
			}
			continue // property: la riempie BindProps, dig non deve vederla
		}
		if f.PkgPath != "" {
			if hasInject || hasFrom {
				return nil, fmt.Errorf("campo %s: i tag %q/%q richiedono un campo esportato", f.Name, InjectTag, FromTag)
			}
			continue // non esportato: campo di lavorazione
		}
		if !hasInject && !hasFrom && !isOptional {
			continue // nessun tag: campo di lavorazione, dig non lo vede
		}
		if hasFrom && injectVal != "" {
			return nil, fmt.Errorf("campo %s: %q non ammette un nome insieme a %q (dig non supporta i value group named)",
				f.Name, FromTag, InjectTag)
		}
		if hasFrom && fromVal == "" {
			return nil, fmt.Errorf("campo %s: il tag %q richiede il nome del value group", f.Name, FromTag)
		}

		var parts []string
		if injectVal != "" {
			parts = append(parts, `name:"`+injectVal+`"`)
		}
		if fromVal != "" {
			parts = append(parts, `group:"`+fromVal+`"`)
		}
		if isOptional {
			parts = append(parts, `optional:"true"`)
		}
		tag := reflect.StructTag(strings.Join(parts, " "))

		d := dep{idx: i, tag: tag}
		if checkableDep(f.Type, fromVal, isOptional) {
			// La rendiamo opzionale per dig e la verifichiamo noi, per poter dare un errore che nomina
			// owner/campo/tipo invece del generico makeFuncStub.
			d.tag = reflect.StructTag(strings.TrimSpace(string(tag) + ` optional:"true"`))
			d.checkable = true
		}
		deps = append(deps, d)
	}
	return deps, nil
}

// embedsIn indica se t porta il marker core.In (fx.In), che in una struct data a ProvideStruct è un
// errore: il param object sintetico è quello a portare il marker.
func embedsIn(t reflect.Type) bool {
	for i := 0; i < t.NumField(); i++ {
		if t.Field(i).Type == inType {
			return true
		}
	}
	return false
}

// checkableDep indica se la dipendenza può essere resa opzionale per dig e verificata da noi: serve un
// tipo nil-abile (per distinguere "assente" da "zero legittimo"), nessun value group (dig rifiuta i
// group opzionali) e nessun optional messo dall'app (che vuole proprio poterla omettere).
func checkableDep(ft reflect.Type, group string, optional bool) bool {
	if group != "" || optional {
		return false
	}
	switch ft.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return true
	default:
		return false
	}
}
