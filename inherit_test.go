package core

import (
	"testing"
	"time"
)

type innerBlock struct {
	Nested string
}

type tuningBlock struct {
	Name       string
	Size       int
	Timeout    time.Duration
	Ratio      float64
	Enabled    bool
	Toggle     *bool
	Attempts   *int
	Props      map[string]string
	Topics     []string
	Inner      innerBlock
	unexported string
}

func TestInherit_ZeroEreditaValorizzatoNo(t *testing.T) {
	global := tuningBlock{
		Name:     "globale",
		Size:     100,
		Timeout:  time.Minute,
		Ratio:    2,
		Toggle:   ptrTo(true),
		Attempts: ptrTo(5),
		Props:    map[string]string{"a": "1", "b": "2"},
		Topics:   []string{"t1"},
		Inner:    innerBlock{Nested: "globale"},
	}
	local := tuningBlock{
		Size:     7,                                // valorizzato: vince
		Props:    map[string]string{"b": "locale"}, // fusa, con il locale a vincere
		Attempts: ptrTo(0),                         // "spegnente" esplicito: NON deve ereditare
	}

	Inherit(&local, &global)

	if local.Size != 7 {
		t.Errorf("Size = %d, atteso 7 (il valore locale vince)", local.Size)
	}
	if local.Name != "globale" || local.Timeout != time.Minute || local.Ratio != 2 {
		t.Errorf("campi non valorizzati non ereditati: %+v", local)
	}
	if local.Toggle == nil || !*local.Toggle {
		t.Error("puntatore nil non ereditato")
	}
	// È la ragione per cui i campi "spegnenti" sono puntatori: uno zero scritto a mano deve
	// sopravvivere a un globale valorizzato.
	if local.Attempts == nil || *local.Attempts != 0 {
		t.Errorf("Attempts = %v, atteso lo 0 esplicito conservato", local.Attempts)
	}
	if local.Props["a"] != "1" {
		t.Error("le chiavi comuni della mappa globale sono state perse")
	}
	if local.Props["b"] != "locale" {
		t.Errorf("Props[b] = %q, atteso il valore locale sui conflitti", local.Props["b"])
	}
	if len(local.Topics) != 1 || local.Topics[0] != "t1" {
		t.Errorf("slice vuota non ereditata: %v", local.Topics)
	}
	if local.Inner.Nested != "globale" {
		t.Error("struct annidata non ereditata")
	}
}

func TestInherit_NonMutaIlGlobale(t *testing.T) {
	// Il blocco globale è condiviso da tutti i discendenti: se l'eredità lo mutasse, le chiavi
	// aggiunte da un processor comparirebbero anche negli altri.
	global := tuningBlock{Props: map[string]string{"a": "1"}}
	a := tuningBlock{Props: map[string]string{"solo-a": "x"}}
	b := tuningBlock{}

	Inherit(&a, &global)
	Inherit(&b, &global)

	if len(global.Props) != 1 {
		t.Errorf("mappa globale mutata: %v", global.Props)
	}
	if _, leaked := b.Props["solo-a"]; leaked {
		t.Error("una chiave di un discendente è passata a un altro attraverso il globale")
	}
}

func TestInherit_SliceValorizzataSostituisce(t *testing.T) {
	// Una lista non si fonde: ordine e completezza appartengono al livello che la scrive.
	global := tuningBlock{Topics: []string{"g1", "g2"}}
	local := tuningBlock{Topics: []string{"l1"}}
	Inherit(&local, &global)
	if len(local.Topics) != 1 || local.Topics[0] != "l1" {
		t.Errorf("Topics = %v, attesa la sola lista locale", local.Topics)
	}
}

func TestInherit_BoolNonEredita(t *testing.T) {
	// false è indistinguibile da "non scritto": chi deve ereditare un booleano usa *bool.
	global := tuningBlock{Enabled: true}
	local := tuningBlock{}
	Inherit(&local, &global)
	if local.Enabled {
		t.Error("un bool non deve ereditare: usare *bool dove serve")
	}
}

func TestIsZeroStruct(t *testing.T) {
	if !IsZeroStruct(tuningBlock{}) {
		t.Error("una struct vuota deve essere zero")
	}
	// Una mappa vuota ma non nil non è un blocco scritto (differenza voluta da reflect.IsZero).
	if !IsZeroStruct(tuningBlock{Props: map[string]string{}}) {
		t.Error("una mappa vuota non conta come blocco valorizzato")
	}
	for name, v := range map[string]tuningBlock{
		"string":          {Name: "x"},
		"int":             {Size: 1},
		"durata":          {Timeout: time.Second},
		"float":           {Ratio: 0.5},
		"bool":            {Enabled: true},
		"puntatore":       {Toggle: ptrTo(false)},
		"mappa":           {Props: map[string]string{"a": "1"}},
		"slice":           {Topics: []string{"t"}},
		"struct annidata": {Inner: innerBlock{Nested: "x"}},
	} {
		if IsZeroStruct(v) {
			t.Errorf("%s valorizzato: IsZeroStruct deve essere false", name)
		}
	}
}

func TestInherit_TipoNonStructPanica(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("atteso panic su un tipo non struct")
		}
	}()
	n, m := 1, 2
	Inherit(&n, &m)
}

func ptrTo[T any](v T) *T { return &v }
