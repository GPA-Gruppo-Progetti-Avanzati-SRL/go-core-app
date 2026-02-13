package authorization

// Authorizer espone il controllo e la consultazione autorizzazioni tra ruoli e funzioni.
// N.B. Il match riguarda solo gli endpoint (operationId).
type Authorizer interface {
	// Match verifica l'autorizzazione su un endpoint identificato da operationId.
	Match(roles []string, operationId string) bool

	// GetCapabilities restituisce l'elenco degli id capability abilitati per i ruoli.
	GetCapabilities(roles []string, appId string) []string

	// GetMenu restituisce l'albero dei menu autorizzati per i ruoli.
	GetMenu(roles []string, appId string) []*MenuNode

	HasCapability(roles []string, capabilityId string) bool

	GetApps(roles []string) []*App
}

// MenuNode rappresenta un nodo di menu nell'albero autorizzato.
type MenuNode struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Order       int    `json:"order,omitempty"`
	Path        string `json:"path,omitempty"`
}

type App struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Order       int    `json:"order,omitempty"`
}
