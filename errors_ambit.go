package core

// Errors costruisce gli ApplicationError di una libreria con il suo ambito già impostato. Ogni
// modulo go-core ne dichiara uno solo:
//
//	var liberr = core.Errors{Ambit: Ambit}
//
//	return liberr.Tech(CodeAcquire).WithCause(err)
//	return liberr.NotFound().WithCause(mongo.ErrNoDocuments)
//
// Serve perché l'ambito è l'unico campo che si può dimenticare senza che nulla lo segnali: i
// costruttori base lo riempiono con AppName, quindi un errore nato in una libreria senza
// WithAmbit si presenta come un errore dell'applicazione che lo riceve. Prima ogni libreria
// aveva i suoi due helper scritti a mano (mongo, sql, batch) oppure la catena completa ripetuta
// a ogni sito (locker, auth, kafka).
//
// Ritorna un *ApplicationError nuovo a ogni chiamata, quindi i modificatori (WithMessage,
// WithCause, ...) si compongono come sui costruttori base.
type Errors struct {
	Ambit string
}

// Tech è TechnicalError() con l'ambito della libreria e il codice dato.
func (e Errors) Tech(code string) *ApplicationError {
	return TechnicalError().WithAmbit(e.Ambit).WithCode(code)
}

// Business è BusinessError() con l'ambito della libreria e il codice dato.
func (e Errors) Business(code string) *ApplicationError {
	return BusinessError().WithAmbit(e.Ambit).WithCode(code)
}

// NotFound è NotFoundError() con l'ambito della libreria: codice NOT-FOUND e messaggio di default.
func (e Errors) NotFound() *ApplicationError {
	return NotFoundError().WithAmbit(e.Ambit)
}
