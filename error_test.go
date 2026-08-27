package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var errSentinel = errors.New("sentinel")

type customError struct{ Detail string }

func (c *customError) Error() string { return "custom: " + c.Detail }

func TestUnwrap_TechnicalError(t *testing.T) {
	appErr := TechnicalError().WithCause(errSentinel)

	if got := errors.Unwrap(appErr); got != errSentinel {
		t.Fatalf("Unwrap = %v, atteso %v", got, errSentinel)
	}
	if !errors.Is(appErr, errSentinel) {
		t.Error("errors.Is deve attraversare fino alla sentinella")
	}
	// Il contratto pubblico non cambia: Error() resta il solo Message.
	if appErr.Error() != errSentinel.Error() {
		t.Errorf("Error() = %q, atteso %q", appErr.Error(), errSentinel.Error())
	}
	if appErr.StatusCode != 500 || appErr.Code != "TECH500" {
		t.Errorf("status/code = %d/%s, attesi 500/TECH500", appErr.StatusCode, appErr.Code)
	}
}

func TestUnwrap_BusinessError(t *testing.T) {
	appErr := BusinessError().WithCause(errSentinel)

	if !errors.Is(appErr, errSentinel) {
		t.Error("errors.Is deve attraversare fino alla sentinella")
	}
	if appErr.StatusCode != 422 || appErr.Code != "BUS422" {
		t.Errorf("status/code = %d/%s, attesi 422/BUS422", appErr.StatusCode, appErr.Code)
	}
}

func TestUnwrap_AttraversaUnaCatenaAnnidata(t *testing.T) {
	// Il caso reale: un driver wrappa la sua sentinella, il data layer la converte
	// in ApplicationError, e il chiamante vuole ancora riconoscerla.
	wrapped := fmt.Errorf("query fallita: %w", errSentinel)
	appErr := TechnicalError().WithCause(wrapped)

	if !errors.Is(appErr, errSentinel) {
		t.Error("errors.Is deve attraversare ApplicationError + fmt.Errorf")
	}
}

func TestErrorsAs_RecuperaIlTipoConcretoDellaCausa(t *testing.T) {
	cause := &customError{Detail: "vincolo violato"}
	appErr := TechnicalError().WithCause(cause)

	var target *customError
	if !errors.As(appErr, &target) {
		t.Fatal("errors.As deve recuperare la causa tipizzata")
	}
	if target.Detail != "vincolo violato" {
		t.Errorf("Detail = %q", target.Detail)
	}

	// errors.AsType è la forma usata dal resto del monorepo (store.ApplyResult,
	// corekafka classify): deve funzionare sulla stessa catena.
	if got, ok := errors.AsType[*customError](error(appErr)); !ok || got.Detail != "vincolo violato" {
		t.Errorf("errors.AsType = (%v, %v)", got, ok)
	}
}

func TestWithCause_ConMessaggioEsplicito(t *testing.T) {
	tests := []struct {
		name   string
		build  func(error) *ApplicationError
		status int
		code   string
	}{
		{
			name: "technical",
			build: func(e error) *ApplicationError {
				return TechnicalError().WithCode("MON-AGGINC").WithMessage("incoerente").WithCause(e)
			},
			status: 500,
			code:   "MON-AGGINC",
		},
		{
			name: "business",
			build: func(e error) *ApplicationError {
				return BusinessError().WithCode("ERR-SORT").WithMessage("sort invalido").WithCause(e)
			},
			status: 422,
			code:   "ERR-SORT",
		},
		{
			name:   "not found",
			build:  func(e error) *ApplicationError { return NotFoundError().WithCause(e) },
			status: 404,
			code:   "NOT-FOUND",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			appErr := tc.build(errSentinel)
			if !errors.Is(appErr, errSentinel) {
				t.Error("WithCause deve rendere la causa raggiungibile da errors.Is")
			}
			if appErr.StatusCode != tc.status || appErr.Code != tc.code {
				t.Errorf("status/code = %d/%s, attesi %d/%s", appErr.StatusCode, appErr.Code, tc.status, tc.code)
			}
			// Il messaggio non viene contaminato dalla causa.
			if appErr.Error() == errSentinel.Error() {
				t.Error("Error() non deve diventare il testo della causa")
			}
		})
	}
}

func TestCausaSintetica_SenzaCausaReale(t *testing.T) {
	tests := []struct {
		name  string
		build func() *ApplicationError
		leaf  string
	}{
		{"technical", func() *ApplicationError { return TechnicalError().WithCode("TECH500").WithMessage("boom") }, "TECH500: boom"},
		{"business", func() *ApplicationError { return BusinessError().WithCode("ERR-SORT").WithMessage("sort invalido") }, "ERR-SORT: sort invalido"},
		{"not found", NotFoundError, "NOT-FOUND: Oggetto non trovato"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			appErr := tc.build()

			// La catena è completa anche senza un errore di partenza: Unwrap non è nil.
			cause := errors.Unwrap(appErr)
			if cause == nil {
				t.Fatal("Unwrap deve ritornare la causa sintetica, non nil")
			}
			if cause.Error() != tc.leaf {
				t.Errorf("causa = %q, attesa %q", cause.Error(), tc.leaf)
			}
			// La foglia non contamina Error(), che resta il solo Message.
			if appErr.Error() != appErr.Message {
				t.Errorf("Error() = %q, atteso %q", appErr.Error(), appErr.Message)
			}
			// Una causa sintetica non fa matchare errori estranei.
			if errors.Is(appErr, errSentinel) {
				t.Error("errors.Is non deve matchare un errore estraneo")
			}
			// errors.Is su sé stesso continua a valere per identità.
			if !errors.Is(error(appErr), error(appErr)) {
				t.Error("errors.Is su sé stesso deve valere")
			}
		})
	}
}

func TestWithCause_VinceSullaFogliaSintetica(t *testing.T) {
	appErr := TechnicalError().WithCode("MON-AGGINC").WithMessage("incoerente").WithCause(errSentinel)

	if got := errors.Unwrap(appErr); got != errSentinel {
		t.Errorf("Unwrap = %v, attesa la causa reale %v", got, errSentinel)
	}
	var synth *codeError
	if errors.As(error(appErr), &synth) {
		t.Error("la causa sintetica deve essere stata sostituita, non affiancata")
	}
}

func TestLog_StampaSoloLeCauseReali(t *testing.T) {
	prev := log.Logger
	t.Cleanup(func() { log.Logger = prev })

	logged := func(appErr *ApplicationError) string {
		var buf bytes.Buffer
		log.Logger = zerolog.New(&buf)
		appErr.Log(opName("test"))
		return buf.String()
	}

	// Causa sintetica: ripeterebbe Message, non va nella riga.
	if out := logged(TechnicalError().WithCode("TECH500").WithMessage("boom")); strings.Contains(out, `"cause"`) {
		t.Errorf("la causa sintetica non deve comparire nel log: %s", out)
	}
	// Causa reale: è l'unico punto in cui l'errore originale è osservabile.
	out := logged(TechnicalError().WithCode("MON-AGGINC").WithMessage("incoerente").WithCause(errSentinel))
	if !strings.Contains(out, `"cause":"sentinel"`) {
		t.Errorf("la causa reale deve comparire nel log: %s", out)
	}
}

type opName string

func (o opName) String() string { return string(o) }

func TestSerializzazioneInvariata(t *testing.T) {
	// La causa non deve comparire nel payload: il body di risposta e il documento
	// persistito devono restare identici a prima dell'introduzione del campo.
	senza := TechnicalError().WithCode("TECH500").WithMessage("boom")
	con := TechnicalError().WithCode("TECH500").WithMessage("boom").WithCause(errSentinel)

	jSenza, err := json.Marshal(senza)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	jCon, err := json.Marshal(con)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(jSenza) != string(jCon) {
		t.Errorf("la causa è finita nel JSON:\nsenza: %s\ncon:   %s", jSenza, jCon)
	}
	if string(jCon) != `{"ambit":"","code":"TECH500","message":"boom"}` {
		t.Errorf("payload inatteso: %s", jCon)
	}
}

func TestApplicationErrorAnnidato_ilPiuEsternoVince(t *testing.T) {
	// errors.As si ferma al primo *ApplicationError della catena: è ciò su cui
	// contano coreapi.configureError e ManageBusinessError.
	inner := BusinessError().WithCode("ERR-PAGESIZE").WithMessage("pagesize invalido")
	outer := BusinessError().WithCause(inner)

	var found *ApplicationError
	if !errors.As(error(outer), &found) {
		t.Fatal("errors.As deve trovare un ApplicationError")
	}
	if found.Code != "BUS422" {
		t.Errorf("Code = %q, atteso quello esterno BUS422", found.Code)
	}
	// Ma quello interno ora è comunque raggiungibile, cosa che prima non era vera.
	if !errors.Is(outer, error(inner)) {
		t.Error("l'ApplicationError interno deve restare raggiungibile nella catena")
	}
}

func TestWithCause_RiempieIlMessaggioSoloSeVuoto(t *testing.T) {
	// Passare l'errore rende WithMessage superfluo nel caso comune.
	if got := TechnicalError().WithCause(errSentinel).Message; got != "sentinel" {
		t.Errorf("Message = %q, atteso quello della causa", got)
	}
	// Un messaggio esplicito vince, in entrambi gli ordini: WithCause riempie un
	// messaggio vuoto, non lo sovrascrive mai.
	prima := TechnicalError().WithMessage("mio").WithCause(errSentinel).Message
	dopo := TechnicalError().WithCause(errSentinel).WithMessage("mio").Message
	if prima != "mio" || dopo != "mio" {
		t.Errorf("l'ordine non deve contare: WithMessage-poi-WithCause=%q, WithCause-poi-WithMessage=%q", prima, dopo)
	}
	// NotFoundError ha già un messaggio: la causa non lo tocca.
	nf := NotFoundError().WithCause(errSentinel)
	if nf.Message != "Oggetto non trovato" {
		t.Errorf("NotFoundError.Message = %q, atteso quello di default", nf.Message)
	}
	if !errors.Is(nf, errSentinel) {
		t.Error("la causa deve comunque essere allegata")
	}
}

func TestModificatoriOrtogonali(t *testing.T) {
	appErr := BusinessError().
		WithCode("ERR-X").
		WithMessage("messaggio").
		WithAmbit("Utils").
		WithCause(errSentinel)

	if appErr.StatusCode != 422 {
		t.Errorf("StatusCode = %d, atteso 422 dal costruttore base", appErr.StatusCode)
	}
	if appErr.Code != "ERR-X" || appErr.Message != "messaggio" || appErr.Ambit != "Utils" {
		t.Errorf("campi = %+v", *appErr)
	}
	if !errors.Is(appErr, errSentinel) {
		t.Error("la causa deve essere raggiungibile")
	}
}

func TestCausaSintetica_RifletteCodeEMessageCorrenti(t *testing.T) {
	// La foglia è sintetizzata da Unwrap, non memorizzata: segue i modificatori
	// applicati dopo la costruzione.
	appErr := TechnicalError().WithMessage("boom").WithCode("SEQ-INV")
	if got := errors.Unwrap(appErr).Error(); got != "SEQ-INV: boom" {
		t.Errorf("foglia = %q, attesa \"SEQ-INV: boom\"", got)
	}
}
