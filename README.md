# GO-CORE-APP

## Installation

    go get github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app

---

Libreria base delle applicazioni GPA (package `core`): boot, configurazione YAML, error handling,
dependency injection su [uber/fx](https://uber-go.github.io/fx/), logging zerolog, validazione,
metriche OpenTelemetry/Prometheus, tracing, CLI cobra, HTTP client strumentato.

È la dipendenza comune di tutti gli altri moduli go-core: `go-core-api`, `go-core-mongo`,
`go-core-sql`, `go-core-redis`, `go-core-batch`, `go-core-kafka`.

**Richiede Go 1.27+.**

---

## Boot dell'app

`core.Boot[A, S](core.App{...}) *S` è il preambolo di ogni app: identità, build info, lettura e
validazione della config (che configura anche il logging), fail-fast su `MODE` non ammesso, e il
`core.Supply` della sezione applicativa `*A`. Ritorna la sola sezione `services`, quella che si
spacchetta nei `*.Module`.

Il boot sta in **due file**: `main.go` (composition root) e `services/config.go` (soli Config).

```go
// main.go
func main() {
    svc := core.Boot[app.Config, services.Config](core.App{
        Name:         app.AppName,
        Logo:         appLogo,      // //go:embed app-logo.txt
        ConfigFile:   configFile,   // //go:embed config.yml
        ConfigEnvVar: "APP_CONFIG_FILE_PATH",
        Modes:        []string{engine.Api, engine.Worker}, // vuoto = app single-mode
    })

    coremongo.Module(&svc.Mongo, coremongo.WithAuthorization())
    coreapi.Module(&svc.Api, coreapi.WithRoutes(routes.Register))

    core.Run(core.WithTracing())
}
```

```go
// services/config.go — SOLO i Config dei servizi, nessun wiring.
type Config struct {
    Mongo coremongo.Config `yaml:"mongo" mapstructure:"mongo"`
    Api   coreapi.Config   `yaml:"api"   mapstructure:"api"`
}
```

I type param di `Boot` sono **posizionali**: prima la config applicativa, poi quella dei servizi.
Invertirli non compila appena si usa il valore di ritorno. Su misconfig `Boot` fa `log.Fatal`:
l'app non parte, invece di girare con default silenziosi.

`core.Run(opts ...RunOption)` chiude il `main`. Le RunOption sostituiscono gli `Invoke` a mano:

| RunOption | Effetto |
|---|---|
| `core.WithTracing(modes...)` | esportazione OTLP delle trace |
| `core.WithServerMetrics(modes...)` | server ops su `0.0.0.0:2112` (`/metrics`, `/health`; `/debug/pprof/*` solo con `metrics.pprof: true`) |

> `WithServerMetrics` **non** va usata in mode API: `go-core-api` serve già `/metrics` sulla porta dell'API.

**Niente `app-config.go`, niente `init()` di boot, niente `version.go`.** Regola: `init()` solo per
l'auto-registrazione dei layer che non leggono la config (`core.ProvideAs[IData](NewData)`); tutto
ciò che consuma la config va in `main()`, dove l'ordine config → wiring → run si legge invece di
dipendere dall'ordine di init dei package.

---

## Configurazione

`core.ReadConfig(embeddedYAML, envVarName, target)` carica lo YAML via Viper con sostituzione delle
variabili d'ambiente (`${ENV_VAR}`), configura il logging e valida la struct (tag `validate:`).
Normalmente non si chiama a mano: lo fa `Boot`.

Il sottoalbero `config:` ha **due sole sezioni**, imposte dalla libreria:

```yaml
log:
  level: info
  enableJSON: true
config:
  app:                 # -> il type param A di Boot
    ...
  services:            # -> il type param S di Boot
    mongo: { ... }
    api:   { ... }
```

Le struct usano i tag `yaml:` + `mapstructure:` (+ `validate:` per la validazione all'avvio).

> **Attenzione alle maiuscole:** viper abbassa ricorsivamente le chiavi (`selfFeed:` nello YAML
> arriva come `selffeed`). Per questo `core.Properties` e `BindProps` confrontano le chiavi
> case-insensitive.

---

## Error handling

Tutte le funzioni pubbliche ritornano `(*T, *core.ApplicationError)`. L'`ApplicationError` porta
`StatusCode`, `Ambit`, `Code`, `Message` e una **causa non esportata**.

Il catalogo dei codici emessi da questa libreria è in **[ERRORI.md](ERRORI.md)**. `core.Ambit`
(`"go-core-app"`) è l'ambito da mettere con `WithAmbit` sugli errori nati **dentro** una libreria
go-core: i costruttori base riempiono `Ambit` con l'`AppName`, cioè con l'app che *riceve* l'errore,
quindi senza sovrascriverlo un guasto della libreria si presenta come un errore dell'applicazione e
chi legge il log non sa dove guardare. Ogni lib go-core ha la propria costante `Ambit` e il proprio
`ERRORI.md`.

L'API è **un costruttore base per status + modificatori ortogonali**, uno per campo, componibili in
qualsiasi ordine:

```go
core.TechnicalError()   // HTTP 500, Code TECH500
core.BusinessError()    // HTTP 422, Code BUS422
core.NotFoundError()    // HTTP 404, Code NOT-FOUND, messaggio di default

    .WithCode(code)      // codice applicativo
    .WithMessage(msg)    // messaggio esposto (è ciò che Error() ritorna e ciò che finisce nel body)
    .WithAmbit(ambit)    // ambito, che i costruttori base riempiono con AppName
    .WithCause(err)      // errore originante → raggiungibile da errors.Is / errors.As
```

```go
return core.TechnicalError().WithCause(err)                              // Message = err.Error()
return core.TechnicalError().WithCode("MONGO-GOBF-ERRCUR").WithCause(err)
return core.BusinessError().WithCode("ERR-SORT").WithCause(err)
return core.NotFoundError().WithCause(mongo.ErrNoDocuments)
return core.TechnicalError().WithCode("SEQ-INV").WithMessage("sequence is not an integer")
```

**`WithCause` riempie il messaggio se è vuoto**, quindi passare l'errore rende `WithMessage`
superfluo nel caso comune. Non lo sovrascrive mai: un messaggio esplicito vince sempre, e l'ordine
fra `WithMessage` e `WithCause` è indifferente.

> I costruttori combinatori `TechnicalErrorWithError`, `BusinessErrorWithError`,
> `TechnicalErrorWithCodeAndMessage`, `BusinessErrorWithCodeAndMessage` **non esistono più**.
> Migrazione meccanica: `XxxErrorWithError(e)` → `XxxError().WithCause(e)`,
> `XxxErrorWithCodeAndMessage(c, m)` → `XxxError().WithCode(c).WithMessage(m)`; dove
> `m == e.Error()` le due si fondono in `.WithCause(e)`.

**`Unwrap()` non ritorna mai nil.** Senza di essa la catena si interromperebbe sull'`ApplicationError`:
`errors.Is`/`errors.As` compilerebbero comunque, ma il ramo che ne dipende non verrebbe mai preso —
un bug che non si manifesta come errore. Se non è stata allegata una causa reale, `Unwrap`
**sintetizza** una foglia dai `Code`/`Message` correnti.

Il campo `cause` è **non esportato di proposito**: `encoding/json` e il codec bson ignorano i campi
non esportati, quindi il body della risposta HTTP e il documento persistito restano identici.
`Error()` ritorna il solo `Message`; `ApplicationError.Log` emette la causa reale come campo
strutturato `cause`.

```go
if appErr != nil {
    appErr.Log(op, core.F("id", id), core.F("collection", "people"))
    return appErr
}
```

Nelle API `coreapi.ManageBusinessError()` converte l'`ApplicationError` in una error response Huma.

---

## Dependency Injection (uber/fx)

I costruttori seguono il pattern `NewXxx` con param/result object `core.In` / `core.Out` — alias
esportati di `fx.In`/`fx.Out`, usabili al posto loro per non importare `go.uber.org/fx` nei package
applicativi (devono restare **alias** `=`, non defined type, o dig non riconosce il marker).

| Primitiva | Uso |
|---|---|
| `core.Provide(ctor, modes...)` | costruttore |
| `core.ProvideAs[T](ctor, modes...)` | costruttore fornito come interfaccia `T` |
| `core.ProvideNamed(ctor, name, modes...)` / `ProvideAsNamed[T]` | istanza con nome dig |
| `core.Supply(value, modes...)` | valore già costruito |
| `core.Invoke(fn, modes...)` | esecuzione all'avvio |
| `core.Populate(&target, modes...)` | estrazione di un'istanza dal grafo |

Il parametro variadico `modes` è il **gating**: la registrazione avviene solo se `core.Mode`
(`os.Getenv("MODE")`) è fra quelli indicati; nessun mode = sempre.

### Scope: `Module` e `ModuleClosed`

`core.Module(name, register)` e `core.ModuleClosed(name, register)` raggruppano in un `fx.Module(name)`
tutte le registrazioni fatte dentro `register`. Oltre al namespacing del grafo e dei log fx,
definiscono la **visibilità**:

| Primitiva | Chi la usa | Supply | Provide |
|---|---|---|---|
| `core.Module` | **driver** (mongo, sql, redis) e registrazioni dell'app | privati (`fx.Private`) | esportati |
| `core.ModuleClosed` | **sottosistemi chiusi** (api, kafka, batch) | privati | privati |

Un driver esiste per dare un handle all'app, quindi espone il `*Service` e tiene privata la config.
Un sottosistema chiuso consuma i seam dell'app (rotte+business, `Handler`/`Transformer`,
`ITaskRunner`) e non le espone nulla.

**L'unica config iniettabile in tutto il grafo è quella applicativa**, supplita da `core.Boot` nello
scope root. Dall'esterno verso l'interno attraversa il confine tutto ciò che è esportato a root
(il business dell'app, il `*coremongo.Service`, i runner); dall'interno verso l'esterno nulla — un
seam pubblico si esprime registrandolo *fuori* dallo scope.

Conseguenza voluta: due moduli fratelli possono supplire lo **stesso tipo** senza `duplicate provide`,
e un `Supply` a root convive con l'omonimo privato di un modulo (che vince per i propri costruttori).

`batch.Module`, `coreapi.Module` e `corekafka.Module` usano già `ModuleClosed` internamente — non
ri-avvolgerli a mano.

#### `core.Private` — granularità dentro un `Module`

```go
core.Module("kafka-producer", func() {
    core.Supply(cfg.Server)           // privato: in un Module i Supply lo sono già
    core.Private(driver)              // la driver.Factory resta dentro
    core.Provide(newProducer)         // il servizio esce
})
```

`core.Private(register)` rende privati i soli `Provide` registrati nella closure, mentre il modulo
continua a esportare gli altri. Serve quando il gruppo di registrazioni che compone un servizio
contiene sia il servizio — che l'app inietta — sia i suoi **ingranaggi**, che non le servono e che due
moduli fratelli potrebbero fornire entrambi: senza, l'unica granularità sarebbe il modulo intero (o
tutto esportato con `Module`, o niente con `ModuleClosed`), e un ingranaggio esportato da due
sottoalberi è un `duplicate provide`. In-tree lo usa `corekafka.ProducerModule` per la `driver.Factory`.

Panica fuori da un `Module`/`ModuleClosed`: a root non esiste uno scope da cui nascondersi, e non fare
nulla in silenzio sarebbe la risposta peggiore.

### `core.ProvideStruct` — costruttore sintetizzato

```go
func ProvideStruct[T any, R any](mk func(*T) R, owner string, props Properties, group string, modes ...string)
```

Fornisce a fx un costruttore sintetizzato (`reflect.StructOf` + `MakeFunc`) per il tipo struct `T`:
dig vede solo un param object con le **sole dipendenze**, quindi i campi di configurazione non
richiedono tag DI. È il meccanismo condiviso da `go-core-kafka` (Handler/Transformer) e
`go-core-batch` (task runner).

| Tag sul campo | Trattamento |
|---|---|
| `inject:""` / `inject:"nome"` | dipendenza fx (tradotta in `name:"nome"` per dig) |
| `from:"gruppo"` | dipendenza da un value group (tradotta in `group:"gruppo"`) |
| `optional:"true"` | dipendenza opzionale |
| `prop:"chiave"` (+ `default:`, `validate:`) | property applicativa, mappata da `BindProps` |
| **nessun tag** | campo di lavorazione: ignorato da dig e dal binding, resta a zero |

```go
type mioRunner struct {
    Data    data.IData `inject:""`
    Soglia  int        `prop:"soglia" default:"100" validate:"gt=0"`
    counter int        // campo di lavorazione
}
```

Combinazioni illegali (`prop:` + `inject:`, `from:` con nome, tag DI su campo non esportato) →
**panic al wiring**. Una dipendenza nil-abile mancante produce un errore che nomina owner, campo e
tipo, invece del `reflect.makeFuncStub` di fx. Una struct data a `ProvideStruct` **non deve
embeddare `core.In`**: è un errore al wiring. `core.In` resta quello di sempre nei param object dei
costruttori scritti a mano passati a `core.Provide`.

---

## Properties applicative

`core.Properties` (`map[string]any`) è il blocco di configurazione applicativa — il `properties:` di
un processor Kafka o di un task batch. I valori conservano il tipo YAML nativo; le chiavi sono
risolte **case-insensitive**.

```go
core.BindProps(&target, props)   // mapping sui campi `prop:` (via ProvideStruct nel caso normale)

props.Has("k")
props.GetString("k", "def")
props.GetInt("k", 0)
props.GetBool("k", false)
props.GetDuration("k", 30*time.Second)
```

Il modo raccomandato è il mapping sui campi `prop:`: il decode avviene al wiring, quindi un valore
non convertibile o un `required` mancante fa **fallire l'avvio** invece di degradare in silenzio sul
default. I getter restano per le properties dinamiche, non strutturate.

---

## Eredità fra livelli di configurazione

```go
core.Inherit[T](dst, src *T)   // dst eredita da src, campo per campo
core.IsZeroStruct[T](v T) bool
```

Implementano l'eredità fra due livelli di config **omologhi** (stesso tipo Go): il blocco globale e
quello che lo sovrascrive — `server.consumer` e `processors[].consumer` di go-core-kafka, e ogni
altro default condiviso raffinato per istanza. Regola applicata a ogni campo:

| Tipo | Regola |
|---|---|
| `*T` nil | eredita (ed è il modo di sovrascrivere con un valore "spegnente": `0`, `false`, `1`) |
| stringa vuota, numero `<= 0` | eredita |
| `map` | si **fonde** chiave per chiave (dst vince) |
| `slice` | vuota eredita, altrimenti sostituisce |
| `bool` | **mai** (false è indistinguibile da "non scritto": serve `*bool`) |
| struct | ricorsione |
| altro | **panic** |

Il panic è voluto: il silenzio alternativo è un campo che non eredita senza che nulla lo segnali.
I **default** non passano da qui e restano espliciti — lì il valore è specifico del campo.

---

## Lock distribuito — `core/lock`

`lock.Locker` è la primitiva neutra, indipendente da qualsiasi backend e da qualsiasi scheduler:

```go
type Locker interface {
    Acquire(ctx context.Context, key string, opts ...AcquireOption) (Handle, error)
}

type Handle interface {
    Release(ctx context.Context) error
    Extend(ctx context.Context) error   // rinnova il TTL, ErrLockLost se perso
}
```

Senza opzioni fa **un solo tentativo non bloccante** e ritorna `lock.ErrNotAcquired` in contesa —
la semantica dispatch-dedup su cui si appoggia lo scheduler batch. Le opzioni coprono la mutua
esclusione di una sezione critica:

| Option | Effetto |
|---|---|
| `WithTries(n)` | `n > 1` rende `Acquire` bloccante, con retry sulla contesa |
| `WithRetryDelay(d)` | attesa fra i tentativi |
| `WithExpiry(d)` | TTL del lock (0 = default del backend) |
| `WithWait(total, delay)` | comodità: `Tries = total/delay`, `RetryDelay = delay` |

Le implementazioni sono nei subpackage `locker/` opt-in dei backend:
[`go-core-redis/locker`](../go-core-redis) (redsync/Redlock),
[`go-core-mongo/locker`](../go-core-mongo) (documento lease con TTL),
[`go-core-sql/locker`](../go-core-sql) (tabella `scheduler_locks`).
`go-core-batch` consuma solo l'interfaccia (`batch.WithLocker`) e la adatta a gocron internamente.

---

## Paginazione e sort — `core/page`

```go
paging := page.InitPaging(nil, pageSize, pageNumber, 0)
sort, err := page.ParseSort("name:asc,createdAt:desc")
```

`*page.Paging` porta `PageSize`, `CurrentPage`, `TotalCount`, `TotalPages`, `HasNext`, `HasPrevious`
e li mantiene coerenti (`SetTotalItems`, `IncCurrentPage`, …). È il tipo che i CRUD di go-core-mongo
e go-core-sql riempiono, e che `coreapi.GeneratePageResponse` traduce negli header di risposta.

---

## Autorizzazione — `core/authorization`

`authorization.Authorizer` è l'interfaccia RBAC neutra (`Match`, `MatchRequest`, `GetContexts`,
`GetApps`, `GetCapabilities`, `HasCapability`, …). Due implementazioni:

- **`ConfigAuthorizer`** — regole da file/config (`NewConfigAuthorizerFromFile`, `NewConfigAuthorizerFromConfig`);
- **LUT su Mongo** — `go-core-mongo` con `coremongo.WithAuthorization()`, alimentata dalla collection ACL.

Il middleware che la consuma sta in `go-core-api`.

---

## Validazione

```go
var Validator = validator.New()   // go-playground/validator, traduzione italiana

if appErr := core.ValidateStruct(input); appErr != nil {
    return appErr
}
```

Ritorna un `ApplicationError` con codice `ERR_VALIDATION` che **conserva la causa**: la
`validator.ValidationErrors` originale è recuperabile con `errors.As`, senza parsare il messaggio.
La validazione della config all'avvio passa da qui.

---

## Concorrenza

```go
a, b, appErr := core.ConcurrentTwo(taskA, taskB)                     // due task eterogenei
res, appErr := core.ConcurrentN(items, 8, func(i T) (R, *core.ApplicationError) { ... })
```

`ConcurrentN` esegue `fn` su ogni item con al massimo `concurrency` goroutine in parallelo; i
risultati mantengono l'ordine degli input e viene ritornato il primo errore incontrato.

---

## HTTP client strumentato

```go
client := core.GenerateHttpClientWithInstrumentation("nome-servizio")
ctx = core.AddEndpointNameMetrics("get-person", ctx)
```

Il client porta trace OTel e metriche per endpoint; `AddEndpointNameMetrics` etichetta la chiamata
nel context.

---

## CLI task runner

Per i binari one-shot (non fx-server):

```go
type ITaskRunner interface {
    Run(ctx context.Context) error
}

core.Execute[mioTask]()   // costruisce il comando cobra, flag auto-derivate, esegue e termina
```

`core.TaskConfig` porta la config del task; le flag sono definite automaticamente dai campi.

---

## Metriche, tracing, health

- `core.NewServerMetrics` (via `core.WithServerMetrics`) espone `/metrics` (Prometheus) e `/health`
  su `0.0.0.0:2112`. Indirizzo, `read-header-timeout` e pprof si configurano dalla sezione YAML
  **`metrics:`**, che è **facoltativa** — ometterla dà esattamente `0.0.0.0:2112` con pprof spento:

  ```yaml
  metrics:
    host: 0.0.0.0            # default: tutte le interfacce (Prometheus scrapa l'IP del pod)
    port: 2112               # default
    pprof: false             # default: /debug/pprof/* NON registrato
    read-header-timeout: 5s  # default
  ```

  Con `pprof: true` il server monta `core.ProfilingHandler()`, quindi `/debug/pprof/goroutineleak`
  (profilo GA da Go 1.27) diventa raggiungibile su `:2112` e mostra le label del framework
  (`batch_job`, `batch_worker`, `kafka_consumer`). Non è un blank import: `_ "net/http/pprof"`
  registrerebbe su `http.DefaultServeMux`, che questo server non è — gli handler sarebbero
  irraggiungibili e insieme pronti a diventare pubblici se una dipendenza servisse quel mux.
  **In mode API il gate è un altro**: lì la porta è quella pubblica dell'API e pprof si accende
  solo con `develop-mode: true` di `go-core-api`.
- `core.NewTracer` (via `core.WithTracing`) configura l'export OTLP.
- `GOMEMLIMIT` è impostato automaticamente dai limiti del cgroup quando l'app gira in container.

---

## Build-time injection

Le build iniettano i metadati di versione direttamente sulle var di questo package, **non** su
quelle di `main`:

```
-X github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app.BuildVersion=$(cat VERSION) \
-X github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app.SHA=$(cat SHA) \
-X github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app.BuildDate=`date +%Y-%m-%dT%T%z`
```

L'app quindi **non** dichiara var `BuildVersion`/`SHA`/`BuildDate` in `package main` e non le
riassegna: farlo azzererebbe il valore messo dal linker. `AppName` non si inietta affatto, lo
imposta `core.Boot`. Se i ldflags mancano (build locale), `Boot` riempie i campi vuoti da
`debug.ReadBuildInfo()`. I valori sono stampati all'avvio insieme a versione di Go, OS, architettura,
`NumCPU`, `GOMAXPROCS` e `GOMEMLIMIT`.

---

## Utility

`core.Encrypt`/`core.Decrypt` (AES), le conversioni data/ora (`StringToDate`, `DateToString`,
`NowTime`, `GetMidnight`, …), `core.GetHostname`, `core.FormatBytes`.

---

## Comandi

```bash
go build ./...
go test ./...
go test -race -count=2 ./...
go vet ./...
```
