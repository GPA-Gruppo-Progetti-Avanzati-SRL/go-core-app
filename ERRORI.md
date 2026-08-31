# Codici di errore — go-core-app

Tutti gli errori pubblici sono `*core.ApplicationError` (`error.go`): `StatusCode`, `Ambit`,
`Code`, `Message` e una causa non esportata (`WithCause` → `errors.Is`/`errors.As`). `Error()`
ritorna il solo `Message`; `Log()` emette la causa reale nel campo strutturato `cause`.

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
| `ERR_VALIDATION` | 500 | `core.ErrValidation` | `validator.go:38` | validazione `validate:` fallita. Il messaggio elenca i campi; la `validator.ValidationErrors` originale è allegata come causa (`errors.As`, niente parsing del testo) |
| `ERR-PAGECFG` | 500 | `page.ErrPageConfig` | `page/appconfig.go:34` | config di paginazione non valida: `default-pagesize` e `default-pagenumber` devono essere > 0 |
| `ERR-PAGESIZE` | 422 | `page.ErrPageSize` | `page/pagingMetaData.go:232` | page size fuori dal dominio ammesso (`< -1`); il messaggio riporta il valore |
| `ERR-PAGESIZE-MAX` | 422 | `page.ErrPageSizeMax` | `page/pagingMetaData.go:241` | page size oltre il massimo (per istanza, o `FallbackMaxPageSize`); il messaggio riporta valore e limite |
| `ERR-PAGENUMBER` | 422 | `page.ErrPageNumber` | `page/pagingMetaData.go:252` | page number < 1; il messaggio riporta il valore |
| `ERR-DATE` | 422 | `core.ErrDateParse` | `utils.go:68` (`StringToDate`) | stringa non conforme a `DateFormat`; l'errore di `time.ParseInLocation` è la causa |

### Cambiamenti rispetto al censimento precedente

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

## Errori sentinella (non `ApplicationError`)

| Simbolo | Package | Significato |
|---|---|---|
| `lock.ErrNotAcquired` | `go-core-app/lock` | lock distribuito già tenuto da un'altra replica: contesa, **non** un guasto |
| `lock.ErrLockLost` | `go-core-app/lock` | lease scaduto o perso durante il rinnovo: il lavoro in corso non è più protetto |

Le implementazioni (`go-core-redis/locker`, `go-core-mongo/locker`, `go-core-sql/locker`)
ritornano questi due e nient'altro di specifico; gli errori di backend risalgono avvolti, da
confrontare con `errors.Is`.

## Errori senza codice (censiti, di proposito non codificati)

Non hanno codice perché **non esiste un chiamante da informare**: o fermano il boot, o sono
errori di programmazione.

| Categoria | Dove | Comportamento |
|---|---|---|
| Config non leggibile / non valida | `config.go:46`, `authorization/loader.go:19,23,34,130,133` | errore risalito a `core.Boot` → `log.Fatal`, l'app non parte |
| Tag DI illegali o struct non sintetizzabile | `modules_synth.go:63,93,99,147,153,177,190,197,205,209` | **panic al wiring** (`prop:`+`inject:`, `from:` con nome, tag su campo non esportato, `core.In` in una struct data a `ProvideStruct`, dipendenza mancante nel grafo) |
| Binding delle properties | `props.go:117,139,175,178,191` | errore risalito dal wiring: property non convertibile o campo `prop:` non esportato |
| Eredità fra livelli di config | `inherit.go:47,61,108,170` | **panic**: tipo non gestito da `core.Inherit`/`core.IsZeroStruct`. Il silenzio alternativo sarebbe un campo che non eredita senza che nulla lo segnali |
| Registrazione delle metriche | `metrics.go:60,70` | `error` (non più **panic**): `NewServerMetrics` è un invoke fx, quindi l'errore ferma l'avvio. Il collector duplicato non si verifica più — il MeterProvider è inizializzato una volta sola per processo (`initMeterProvider`), perché il registry Prometheus è globale |
| Avvio del server ops | `metrics.go:122` | `error`: bind fallito su `metrics.host:port` (porta occupata). Prima l'esito di `ListenAndServe` finiva in un blocco vuoto e il processo restava "sano" senza servire nulla |
| Parsing del `sort` | `page/sort.go:46,56` | `error` semplice, ritornato a chi chiama `page.ParseSort`; go-core-api lo trasforma in `ERR-SORT` |
| Cifratura | `crypt.go:60` | `ciphertext too short`: `error` semplice |
| Paginazione incoerente | `page/pagingMetaData.go:195,205` | **panic** `invalid current page`: stato interno impossibile, non un input utente |
