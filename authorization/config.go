package authorization

// AuthRulesConfig contiene le regole di autorizzazione: gruppi e ruoli.
// Apps e Contexts sono nel gateway config (config.yml).
// Le capability (ui, action_ui, api, action_api) sono scoperte al boot dai file generati:
//   - ui, action_ui  → {static-root}/capabilities-manifest.json  (Angular build)
//   - api, action_api → proxy.capabilities-file                   (backend go-core-api)
type AuthRulesConfig struct {
	CapabilityGroups []CapGroupRuleConfig `yaml:"capability-groups" mapstructure:"capability-groups"`
	Roles            []RoleRuleConfig     `yaml:"roles"             mapstructure:"roles"`
}

// ContextRuleConfig contiene i metadati di un contesto.
// HomeApp indica l'ID dell'app home dedicata a questo contesto.
// Il controllo di accesso è derivato da roles[].context, non da allowed-roles.
type ContextRuleConfig struct {
	ID      string `yaml:"id"       mapstructure:"id"       validate:"required"`
	Label   string `yaml:"label"    mapstructure:"label"`
	HomeApp string `yaml:"home-app" mapstructure:"home-app"` // app-id dell'home di questo contesto
}

// AppRuleConfig descrive un'app: metadati ACL usati dall'authorizer.
// L'associazione contesto → home app è dichiarata in ContextRuleConfig.HomeApp.
type AppRuleConfig struct {
	ID          string `yaml:"id"          mapstructure:"id"          validate:"required"`
	Description string `yaml:"description" mapstructure:"description"`
	BasePath    string `yaml:"base-path"   mapstructure:"base-path"` // "/" per home, "/nome-app/" per le altre
	Icon        string `yaml:"icon"        mapstructure:"icon"`
	Order       int    `yaml:"order"       mapstructure:"order"`
}

// RouteRuleConfig associa un path pattern ai ruoli richiesti.
// Usata dal RouteMatcher statico come fallback dell'AuthorizationMiddleware.
// In presenza di ConfigAuthorizer il MatchRequest deriva dalle capability "api"
// e questo tipo è riservato a override manuali nel gateway config.
type RouteRuleConfig struct {
	Path         string   `yaml:"path"          mapstructure:"path"          validate:"required"`
	Methods      []string `yaml:"methods"       mapstructure:"methods"`
	AllowedRoles []string `yaml:"allowed-roles" mapstructure:"allowed-roles"`
}

// CapabilityRuleConfig definisce una singola capability.
// Distinta per category:
//
//	"api"        → ApiNode  (GetServerCapabilities, Match, MatchRequest)
//	"ui"         → UINode   (GetPaths, filtrato per app-id)
//	"action_ui"  → ActUi    (GetCapabilities, filtrato per app-id)
//	"action_api" → ActApi   (HasCapability)
type CapabilityRuleConfig struct {
	ID          string               `yaml:"id"          mapstructure:"id"          validate:"required"`
	Category    string               `yaml:"category"    mapstructure:"category"`
	Description string               `yaml:"description" mapstructure:"description"`
	API         *CapabilityAPIConfig `yaml:"api"         mapstructure:"api"`
	// Campi per category: "ui" e "action_ui"
	AppID    string `yaml:"app-id"   mapstructure:"app-id"`
	Icon     string `yaml:"icon"     mapstructure:"icon"`
	Order    int    `yaml:"order"    mapstructure:"order"`
	Endpoint string `yaml:"endpoint" mapstructure:"endpoint"`
	IsMenu   bool   `yaml:"menu"     mapstructure:"menu"`
}

// CapabilityAPIConfig descrive l'endpoint HTTP per le capability di categoria "api".
type CapabilityAPIConfig struct {
	OperationID string   `yaml:"operationid" mapstructure:"operationid"` // uso backend (go-core-api Match)
	Path        string   `yaml:"path"        mapstructure:"path"`        // uso gateway (MatchRequest)
	Methods     []string `yaml:"methods"     mapstructure:"methods"`
}

// CapGroupRuleConfig raggruppa capability logicamente correlate.
type CapGroupRuleConfig struct {
	ID           string   `yaml:"id"           mapstructure:"id"           validate:"required"`
	Capabilities []string `yaml:"capabilities" mapstructure:"capabilities"`
}

// RoleRuleConfig mappa un ruolo ai suoi capability group e/o capability dirette.
// Context vuoto = ruolo context-agnostic (FilterRolesByContext lo include sempre).
type RoleRuleConfig struct {
	ID               string   `yaml:"id"                mapstructure:"id"                validate:"required"`
	Context          string   `yaml:"context"           mapstructure:"context"` // vuoto = context-agnostic
	CapabilityGroups []string `yaml:"capability-groups" mapstructure:"capability-groups"`
	Capabilities     []string `yaml:"capabilities"      mapstructure:"capabilities"` // capability dirette senza gruppo
}
