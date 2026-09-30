# Codici di errore — go-core-app

Tutti gli errori pubblici sono `*core.Error` (`errors.go`): `StatusCode`, `Ambit`,
`Code`, `Message` e una causa non esportata (`WithCause` → `errors.Is`/`errors.As`). `Error()`
ritorna il `Message` (o il `Code`, se il messaggio è vuoto); `Log()` emette la causa reale nel campo strutturato `cause`.

> **`Ambit` dice da quale libreria viene l'errore.** I costruttori base (`TechnicalError()`,
> `BusinessError()`, `NotFoundError()`) riempiono `Ambit` con `AppName`, cioè con l'app che
> l'errore lo **riceve**. Un errore nato dentro una libreria go-core lo sovrascrive con la
> propria costante `Ambit` — `core.Ambit` = `"go-core-app"`, `coremongo.Ambit`,
> `coresql.Ambit`, `coreapi.Ambit`, `errs.Ambit` di batch — altrimenti un guasto della
> libreria si presenta come un errore dell'applicazione e chi legge il log non sa dove
> guardare.

## Codici di default dei costruttori

| Codice | HTTP | Costruttore | Quando |
|---|---|---|---|
| `TECH500` | 500 | `core.TechnicalError()` | errore tecnico senza codice specifico |
| `BUS422` | 422 | `core.BusinessError()` | errore applicativo senza codice specifico |
| `NOT-FOUND` | 404 | `core.NotFoundError()` | oggetto non trovato; messaggio di default `Oggetto non trovato` |

Dopo il censimento **nessun sito della libreria ricade più sul default**: i tre codici restano
per il codice applicativo che li usa direttamente.

## Codici emessi dal modulo

Ambit: `go-core-app` (costante `core.Ambit`).

| Codice | HTTP | Costante | Origine | Significato |
|---|---|---|---|---|
| `ERR_VALIDATION` | 500 | `core.ErrValidation` | `validate.go` | validazione `validate:` fallita. Il messaggio elenca i campi; la `validator.ValidationErrors` originale è allegata come causa (`errors.As`, niente parsing del testo) |
| `ERR-PAGECFG` | 500 | `page.ErrPageConfig` | `page/appconfig.go:34` | config di paginazione non valida: `default-pagesize` e `default-pagenumber` devono essere > 0 |
| `ERR-PAGESIZE` | 422 | `page.ErrPageSize` | `page/pagingMetaData.go` (`validatorPageSize`) | page size fuori dal dominio ammesso (`< 0`: `-1` è la sentinella di `InitPaging` e lì si risolve nel default); il messaggio riporta il valore |
| `ERR-PAGESIZE-MAX` | 422 | `page.ErrPageSizeMax` | `page/pagingMetaData.go:241` | page size oltre il massimo (per istanza, o `FallbackMaxPageSize`); il messaggio riporta valore e limite |
| `ERR-PAGENUMBER` | 422 | `page.ErrPageNumber` | `page/pagingMetaData.go:252` | page number < 1; il messaggio riporta il valore |
| `CONCURRENT-PANIC` | 500 | `utils.ErrConcurrentPanic` | `utils/concurrent.go` (`call`) | un task di `ConcurrentTwo`/`ConcurrentN` è andato in panic; il valore del panic e lo stack sono la causa. Prima il panic in una goroutine della libreria terminava il processo |
| `ERR-DATE` | 422 | `utils.ErrDateParse` | `utils/dates.go` (`StringToDate`) | stringa non conforme a `DateFormat`; l'errore di `time.ParseInLocation` è la causa |

### Cambiamenti rispetto al censimento precedente

- **2026-09-30, split del modulo** — i codici non cambiano. Errori e validazione restano nella
  radice (`core.ErrValidation`, `core.Ambit`), il tipo si chiama ora **`core.Error`** (era
  `core.ApplicationError`); `ERR-DATE` → `utils.ErrDateParse`, `CONCURRENT-PANIC` →
  `utils.ErrConcurrentPanic`; `page.*` invariati.
- **2026-09-30** — `CONCURRENT-PANIC` nuovo. `ERR-PAGESIZE` rifiuta anche `-1` su un `Paging`
  già costruito (dava `TotalPages` e offset negativi). `Error()` ritorna il `Code` quando il
  `Message` è vuoto. Le librerie costruiscono i propri errori con **`core.AmbitErrors{Ambit}`**
  (`Tech`/`Business`/`NotFound`), che sostituisce gli helper `techErr`/`notFound` scritti a mano e
  le catene `TechnicalError().WithAmbit(...).WithCode(...)` ripetute a ogni sito.
  **Rimossi** (breaking): `page.Page`/`PagingItems` (paginazione in memoria, panicava con
  `pageSize` 0 o pagina fuori range; la paginazione si fa con `Paging.Paging()`) e
  `core.ConvertStringToTimeDate` (duplicato di `StringToDate` che accettava `2026-13-40`).

- **`99999` non esiste più**: era un segnaposto che non diceva nulla a chi lo riceveva →
  `ERR-DATE`. L'`Ambit` era `"Utils Methods - StringToDate"`; ora è `go-core-app` e il
  contesto sta nel messaggio.
- **`ERR-PAGESIZE` era usato per due condizioni diverse** (valore illegale, valore oltre il
  massimo) con lo stesso messaggio fisso `invalid page size`: la seconda ha ora
  `ERR-PAGESIZE-MAX` e entrambe riportano i numeri in gioco.
- **`Paging()`, `SetPageSize()` e `SetCurrentPage()` non riavvolgono più l'errore** in un
  `BusinessError()` nudo: il wrapping sostituiva `ERR-PAGESIZE`/`ERR-PAGENUMBER` con `BUS422`,
  cioè buttava via il codice appena calcolato. Ora rimbalzano l'errore interno così com'è —
  stesso tipo, codice conservato.

## Errori sentinella del lock

Non sono più qui: `ErrNotAcquired` ed `ErrLockLost` sono di **go-core-locker** (`corelock`), col
resto del lock distribuito — vedi `go-core-locker/ERRORI.md`.

## Errori senza codice (censiti, di proposito non codificati)

Non hanno codice perché **non esiste un chiamante da informare**: o fermano il boot, o sono
errori di programmazione.

| Categoria | Dove | Comportamento |
|---|---|---|
| Config non leggibile / non valida | `config.go` (`ReadConfig`) | errore **restituito** (lettura, decode, `log.level` non valido, validazione) e risalito a `core.Boot` → `log.Fatal`, l'app non parte. Prima `ReadConfig` faceva `log.Fatal` da sé pur dichiarando di ritornare `error` |
| Tag DI illegali o struct non sintetizzabile | `modules.go` (`ProvideStruct`, `synthCtor`, `collectDeps`) | **panic al wiring** (`prop:`+`inject:`, `from:` con nome, tag su campo non esportato, `core.In` in una struct data a `ProvideStruct`, dipendenza mancante nel grafo) |
| Binding delle properties | `properties/props.go` | errore risalito dal wiring: property non convertibile o campo `prop:` non esportato |
| Eredità fra livelli di config | `properties/inherit.go` | **panic**: tipo non gestito da `properties.Inherit`/`properties.IsZeroStruct`. Il silenzio alternativo sarebbe un campo che non eredita senza che nulla lo segnali |
| Registrazione delle metriche | `observability/metrics.go` | `error` (non più **panic**): `NewServerMetrics` è un invoke fx, quindi l'errore ferma l'avvio. Il collector duplicato non si verifica più — il MeterProvider è inizializzato una volta sola per processo (`initMeterProvider`), perché il registry Prometheus è globale |
| Avvio del server ops / dell'API | `httpx/lifecycle.go` (`ServeOnLifecycle`) | `error` da OnStart: bind fallito (porta occupata). Un server che muore **a regime** fa uscire il processo con codice 1 |
| Parsing del `sort` | `page/sort.go:46,56` | `error` semplice, ritornato a chi chiama `page.ParseSort`; go-core-api lo trasforma in `ERR-SORT` |
| Cifratura | `utils/crypt.go` | `ciphertext too short`: `error` semplice |
| Paginazione incoerente | `page/pagingMetaData.go` (`Inc/DecCurrentPage`) | **nessun errore**: `IncCurrentPage` da una pagina `< 1` va alla prima, `DecCurrentPage` non scende sotto 1. Prima era un panic dentro l'handler della richiesta |
