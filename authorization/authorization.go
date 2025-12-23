package authorization

// Authorizer espone il controllo e la consultazione autorizzazioni tra ruoli e funzioni.
// N.B. Il match riguarda solo gli endpoint (operationId).
type Authorizer interface {
	// Match verifica l'autorizzazione su un endpoint identificato da operationId.
	Match(roles []string, operationId string) bool

	// GetCapabilities restituisce l'elenco degli id capability abilitati per i ruoli.
	// appId è opzionale: se valorizzato, limita le capability all'applicazione indicata.
	GetCapabilities(roles []string, appId ...string) []string

	// GetMenu restituisce l'albero dei menu autorizzati per i ruoli.
	// appId è opzionale: se valorizzato, limita il menu all'applicazione indicata.
	GetMenu(roles []string, appId ...string) []*MenuNode

	HasCapability(roles []string, capabilityId string) bool
}

// MenuNode rappresenta un nodo di menu nell'albero autorizzato.
type MenuNode struct {
	ID               string      `json:"id"`
	Description      string      `json:"description,omitempty"`
	IsLeaf           bool        `json:"isleaf,omitempty"`
	Icon             string      `json:"icon,omitempty"`
	Order            int         `json:"order,omitempty"`
	Endpoint         string      `json:"endpoint,omitempty"`
	FunctionParentID string      `json:"functionparentid,omitempty"`
	Children         []*MenuNode `json:"children,omitempty"`
}
