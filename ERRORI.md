# Codici di errore — go-core-app

Tutti gli errori pubblici sono `*core.ApplicationError` (`error.go`): `StatusCode`, `Ambit`
(riempito con `AppName`), `Code`, `Message` e una causa non esportata (`WithCause` →
`errors.Is`/`errors.As`). `Error()` ritorna il solo `Message`; `Log()` emette la causa reale
nel campo strutturato `cause`.

## Codici di default dei costruttori

| Codice | HTTP | Costruttore | Quando |
|---|---|---|---|
| `TECH500` | 500 | `core.TechnicalError()` | errore tecnico senza codice specifico (default: resta questo se non si chiama `WithCode`) |
| `BUS422` | 422 | `core.BusinessError()` | errore applicativo senza codice specifico |
| `NOT-FOUND` | 404 | `core.NotFoundError()` | oggetto non trovato; messaggio di default `Oggetto non trovato` |

## Codici emessi dal modulo

| Codice | HTTP | Origine | Significato / causa allegata |
|---|---|---|---|
| `ERR_VALIDATION` | 500 | `validator.go:38` (`core.ValidateStruct`) | validazione `validate:` fallita. La costante è esportata: `core.ErrValidation`. Il messaggio elenca i campi; la `validator.ValidationErrors` originale è allegata come causa (recuperabile con `errors.As`, niente parsing del testo) |
| `ERR-PAGECFG` | 500 | `page/appconfig.go:25` | config di paginazione non valida: `default-pagesize` e `default-pagenumber` devono essere > 0 |
| `ERR-PAGESIZE` | 422 | `page/pagingMetaData.go:229,237` | page size richiesto non valido (fuori dai limiti o non numerico) |
| `ERR-PAGENUMBER` | 422 | `page/pagingMetaData.go:247` | page number richiesto non valido |
| `99999` | 422 | `utils.go:64` (`core.StringToDate`) | data non parsabile con `DateFormat`. Ambit forzato a `Utils Methods - StringToDate`. **Codice segnaposto**: da rimpiazzare con un codice parlante |

## Errori sentinella (non `ApplicationError`)

| Simbolo | Package | Significato |
|---|---|---|
| `lock.ErrNotAcquired` | `go-core-app/lock` | lock distribuito già tenuto da un'altra replica: contesa, **non** un guasto. Chi lo riceve salta il tick / il lavoro |
| `lock.ErrLockLost` | `go-core-app/lock` | lease scaduto o perso durante il rinnovo: il lavoro in corso non è più protetto |

Le implementazioni di `lock.Locker` (`go-core-redis/locker`, `go-core-mongo/locker`,
`go-core-sql/locker`) ritornano questi due e nient'altro di specifico: gli errori di backend
risalgono avvolti.

## Errori che non passano dai codici

- `core.Boot` / `core.ReadConfig`: misconfig, `MODE` non ammesso, config non valida →
  `log.Fatal`, l'app **non parte**. Non c'è codice perché non c'è chiamante da informare.
- `core.ProvideStruct` / `core.BindProps` / `core.Inherit`: combinazioni di tag illegali,
  `core.In` in una struct sintetizzata, tipo non previsto nell'eredità → **panic al wiring**.
  Sono errori di programmazione, non condizioni di runtime.
