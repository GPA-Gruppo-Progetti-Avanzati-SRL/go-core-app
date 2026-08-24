package core

import (
	"strings"
	"testing"
	"time"
)

type nestedProp struct {
	Inner string `prop:"inner"`
}

// propsTarget raccoglie tutti i tipi supportati dal mapping. In una struct reale questi campi
// convivono con le dipendenze `inject:` e con i campi di lavorazione.
type propsTarget struct {
	Collection string        `prop:"collection" validate:"required"`
	BatchLimit int           `prop:"batch-limit" default:"100"`
	Enabled    bool          `prop:"enabled"`
	Timeout    time.Duration `prop:"timeout" default:"5s"`
	Tags       []string      `prop:"tags"`
	Nested     nestedProp    `prop:"nested"`
	NoTag      string        // nessun tag prop: mai toccato dal mapping
}

// I valori arrivano dal YAML col loro tipo nativo (int, bool, lista, mappa) oppure come stringa (per es.
// dopo una sostituzione ${ENV_VAR}): entrambi i casi devono convertirsi.
func TestBindProps_Types(t *testing.T) {
	var p propsTarget
	err := BindProps(&p, Properties{
		"collection":  "events",
		"batch-limit": 200,                                 // int nativo
		"enabled":     "true",                              // stringa da ${ENV_VAR}
		"timeout":     "1500ms",                            // durata
		"tags":        []any{"a", "b"},                     // lista YAML
		"nested":      map[string]any{"inner": "profondo"}, // mappa annidata
	})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if p.Collection != "events" || p.BatchLimit != 200 || !p.Enabled {
		t.Fatalf("scalari errati: %+v", p)
	}
	if p.Timeout != 1500*time.Millisecond {
		t.Fatalf("durata errata: %v", p.Timeout)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "a" || p.Tags[1] != "b" {
		t.Fatalf("lista errata: %v", p.Tags)
	}
	if p.Nested.Inner != "profondo" {
		t.Fatalf("mappa annidata errata: %+v", p.Nested)
	}
	if p.NoTag != "" {
		t.Fatalf("un campo senza tag prop non deve essere toccato: %q", p.NoTag)
	}
}

// ReadConfig passa da viper, che ABBASSA le chiavi della config: `workType:` nello YAML arriva qui come
// `worktype`. Il lookup deve quindi essere case-insensitive, altrimenti un tag camelCase non matcha mai.
func TestBindProps_KeyLookupIsCaseInsensitive(t *testing.T) {
	var s struct {
		WorkType string `prop:"workType"`
		SelfFeed bool   `prop:"selfFeed"`
	}
	// chiavi come le consegna viper dopo l'unmarshal
	if err := BindProps(&s, Properties{"worktype": "Import", "selffeed": "true"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if s.WorkType != "Import" || !s.SelfFeed {
		t.Fatalf("chiavi abbassate da viper non risolte: %+v", s)
	}
}

func TestProperties_GettersAreCaseInsensitive(t *testing.T) {
	p := Properties{"worktype": "Import", "selffeed": true, "batchlimit": 7, "maxwait": "2s"}
	if !p.Has("workType") {
		t.Fatal("Has deve essere case-insensitive")
	}
	if p.GetString("workType", "") != "Import" {
		t.Fatalf("GetString: %q", p.GetString("workType", ""))
	}
	if !p.GetBool("selfFeed", false) {
		t.Fatal("GetBool")
	}
	if p.GetInt("batchLimit", 0) != 7 {
		t.Fatal("GetInt")
	}
	if p.GetDuration("maxWait", 0) != 2*time.Second {
		t.Fatal("GetDuration")
	}
	if p.GetString("assente", "def") != "def" {
		t.Fatal("chiave assente deve dare il default")
	}
}

// Una lista può anche arrivare come stringa separata da virgole.
func TestBindProps_CommaSeparatedSlice(t *testing.T) {
	var p propsTarget
	if err := BindProps(&p, Properties{"collection": "c", "tags": "a,b,c"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if len(p.Tags) != 3 {
		t.Fatalf("attesi 3 tag, ottenuto %v", p.Tags)
	}
}

// I default del tag valgono solo per le chiavi assenti: quelle presenti vincono.
func TestBindProps_Defaults(t *testing.T) {
	var p propsTarget
	if err := BindProps(&p, Properties{"collection": "c"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if p.BatchLimit != 100 || p.Timeout != 5*time.Second {
		t.Fatalf("default non applicati: %+v", p)
	}

	var q propsTarget
	if err := BindProps(&q, Properties{"collection": "c", "batch-limit": 7, "timeout": "1s"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if q.BatchLimit != 7 || q.Timeout != time.Second {
		t.Fatalf("il valore in config deve vincere sul default: %+v", q)
	}
}

// Un valore presente ma non convertibile è un ERRORE, non un fallback silenzioso al default (era il
// comportamento dei getter untyped: GetInt("batch-limit", 100) su "abc" ritornava 100).
func TestBindProps_UnconvertibleValueFails(t *testing.T) {
	var p propsTarget
	err := BindProps(&p, Properties{"collection": "c", "batch-limit": "abc"})
	if err == nil {
		t.Fatal("atteso errore per un valore non convertibile")
	}
	if !strings.Contains(err.Error(), "batch-limit") {
		t.Fatalf("l'errore deve nominare la property: %v", err)
	}
}

// Chiave non reclamata da nessun campo: ignorata (solo un Warn), non un errore.
func TestBindProps_UnknownKeyIgnored(t *testing.T) {
	var p propsTarget
	if err := BindProps(&p, Properties{"collection": "c", "colection": "typo"}); err != nil {
		t.Fatalf("una chiave sconosciuta non deve bloccare l'avvio: %v", err)
	}
	if p.Collection != "c" {
		t.Fatalf("collection errata: %q", p.Collection)
	}
}

// validate: per campo, incluso il caso senza properties (mappa nil): il required deve scattare.
func TestBindProps_ValidatePerField(t *testing.T) {
	for name, in := range map[string]Properties{
		"chiave assente": {"batch-limit": 1},
		"properties nil": nil,
	} {
		var p propsTarget
		err := BindProps(&p, in)
		if err == nil {
			t.Fatalf("%s: atteso errore di validazione", name)
		}
		if !strings.Contains(err.Error(), "collection") || !strings.Contains(err.Error(), "Collection") {
			t.Fatalf("%s: l'errore deve nominare property e campo: %v", name, err)
		}
	}
}

// Un campo property non deve mai ereditare un valore preesistente (es. arrivato dal grafo fx).
func TestBindProps_ZeroesPreexistingValue(t *testing.T) {
	p := propsTarget{Collection: "c", BatchLimit: 42, Tags: []string{"iniettato"}}
	if err := BindProps(&p, Properties{"collection": "c"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if p.BatchLimit != 100 { // default, non il 42 preesistente
		t.Fatalf("il campo doveva essere azzerato e poi defaultato, ottenuto %d", p.BatchLimit)
	}
	if p.Tags != nil {
		t.Fatalf("il campo doveva essere azzerato, ottenuto %v", p.Tags)
	}
}

// Tag vuoto: la chiave è il nome del campo in minuscolo.
func TestBindProps_EmptyTagUsesFieldName(t *testing.T) {
	var s struct {
		Collection string `prop:""`
	}
	if err := BindProps(&s, Properties{"collection": "events"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if s.Collection != "events" {
		t.Fatalf("chiave dedotta dal nome campo errata: %q", s.Collection)
	}
}

// Una struct che non usa il mapping non deve ricevere errori né warning.
func TestBindProps_NoPropFields(t *testing.T) {
	var s struct {
		In
		Whatever string
	}
	if err := BindProps(&s, Properties{"collection": "events"}); err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if s.Whatever != "" {
		t.Fatalf("nessun campo doveva essere toccato: %+v", s)
	}
}

func TestBindProps_RequiresPointerToStruct(t *testing.T) {
	if err := BindProps(propsTarget{}, nil); err == nil {
		t.Fatal("atteso errore passando un valore non-puntatore")
	}
}

func TestBindProps_PropTagRequiresExportedField(t *testing.T) {
	var s struct {
		hidden string `prop:"hidden"`
	}
	err := BindProps(&s, Properties{"hidden": "x"})
	if err == nil || !strings.Contains(err.Error(), "esportato") {
		t.Fatalf("atteso errore sul campo non esportato: %v", err)
	}
	_ = s.hidden
}
