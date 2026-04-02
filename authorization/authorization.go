package authorization

// Authorizer espone il controllo e la consultazione autorizzazioni tra ruoli e funzioni.
type Authorizer interface {
	// FilterRolesByContext filtra i ruoli per contesto.
	// Restituisce i ruoli con _cid == contextId più i ruoli senza contesto (context-agnostic).
	// Se contextId è vuoto restituisce tutti i ruoli invariati.
	// Un risultato vuoto (con contextId non vuoto) indica che l'utente non è autorizzato per quel contesto.
	FilterRolesByContext(roles []string, contextId string) []string
	// GetContexts restituisce i contesti accessibili per i ruoli passati (usa allRoles).
	GetContexts(roles []string) []*Context
	// GetApps restituisce le app navigabili per i ruoli e il contesto indicati.
	// Se contextID è vuoto restituisce solo le app single-context (l'utente deve ancora selezionare il contesto).
	// Se contextID è valorizzato restituisce le app single-context più le app multi-context per quel contesto.
	// Riceve allRoles (non filtrati) per valutare l'accesso globale.
	GetApps(roles []string, contextID string) []*App
	// MatchRequest verifica l'autorizzazione su una richiesta HTTP identificata da path e method.
	// Il path supporta glob: /api/persons/**, /api/persons/*, /api/persons/:id
	// Usato dal gateway per autorizzazione dinamica senza operationId.
	MatchRequest(roles []string, path, method string) bool
	// GetServerCapabilities restituisce gli id delle capability di tipo "api" (server-side)
	// abilitate per i ruoli. Non scoped all'app — usato dal gateway per iniettare X-Capabilities.
	GetServerCapabilities(roles []string) []string
	// GetCapabilities restituisce l'elenco degli id capability abilitati per i ruoli.
	GetCapabilities(roles []string, appId string) []string
	// GetPaths restituisce l'albero dei menu autorizzati per i ruoli.
	GetPaths(roles []string, appId string) []*Path
	// AllContextIDs restituisce tutti gli ID di contesto disponibili nell'ACL.
	// Usato dal gateway al boot per registrare le route /{cid}/* senza dipendere
	// dai ruoli di uno specifico utente.
	AllContextIDs() []string
	// HomeAppForContext restituisce l'ID dell'app home designata per il contesto indicato.
	// Restituisce stringa vuota se il contesto non ha una home dedicata.
	HomeAppForContext(contextID string) string
}

type Context struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Label       string `json:"label,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Order       int    `json:"order,omitempty"`
}

type Path struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Order       int    `json:"order,omitempty"`
	Menu        bool   `json:"ismenu"`
	Endpoint    string `json:"path,omitempty"`
}

type App struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Order       int    `json:"order,omitempty"`
	ContextID   string `json:"context_id,omitempty"`
}
