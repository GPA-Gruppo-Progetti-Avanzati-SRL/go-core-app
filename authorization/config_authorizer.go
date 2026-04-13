package authorization

import (
	"strings"
)

// ConfigAuthorizer implementa Authorizer usando la configurazione YAML.
// Semantica allineata a AuthorizationLut (go-core-mongo):
//
//	GetServerCapabilities → category "api"
//	GetCapabilities       → category "action_ui", filtrate per appId
//	HasCapability         → category "action_api"
//	GetPaths              → category "ui", filtrate per appId
//	FilterRolesByContext  → usa roleContexts (roles[].context) + agnosticRoles
//	GetContexts           → deriva da roleContexts, label da contextLabels
//	GetApps non-home      → deriva dalle capability "ui" per app-id
//	MatchRequest          → deriva dalle capability "api" (path + methods)
type ConfigAuthorizer struct {
	apps            []appEntry
	caps            map[string]capEntry
	roleCaps        map[string][]string // roleID → []capID
	roleContexts    map[string]string   // roleID → contextID (vuoto = agnostic)
	contextLabels   map[string]string   // contextID → label
	contextHomeApps map[string]string   // contextID → appID dell'home dedicata
	agnosticRoles   map[string]struct{} // ruoli senza context
}

type appEntry struct {
	id          string
	description string
	basePath    string
	icon        string
	order       int
	isHome      bool
}

type capEntry struct {
	id          string
	category    string // "api" | "ui" | "action_ui" | "action_api"
	description string
	operationID string   // api: uso backend (Match)
	apiPath     string   // api: uso gateway (MatchRequest)
	apiMethods  []string // api: metodi HTTP abbinati al path
	icon        string   // ui
	order       int      // ui
	endpoint    string   // ui
	isMenu      bool     // ui
	appID       string   // ui, action_ui
}

// HomeAppForContext restituisce l'ID dell'app home designata per il contesto indicato.
func (a *ConfigAuthorizer) HomeAppForContext(contextID string) string {
	return a.contextHomeApps[contextID]
}

// AllContextIDs restituisce gli ID di tutti i contesti definiti nella configurazione.
// Usato dal gateway al boot per registrare le route /{cid}/*.
func (a *ConfigAuthorizer) AllContextIDs() []string {
	ids := make([]string, 0, len(a.contextLabels))
	for id := range a.contextLabels {
		ids = append(ids, id)
	}
	return ids
}

// FilterRolesByContext restituisce i ruoli compatibili con il contextId.
// Un ruolo è compatibile se il suo context corrisponde a contextId o se è agnostic (context vuoto).
func (a *ConfigAuthorizer) FilterRolesByContext(roles []string, contextId string) []string {
	if contextId == "" {
		return roles
	}
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		if rc, ok := a.roleContexts[role]; ok && rc == contextId {
			result = append(result, role)
			continue
		}
		if _, ok := a.agnosticRoles[role]; ok {
			result = append(result, role)
		}
	}
	return result
}

// GetContexts restituisce i contesti accessibili per i ruoli dati.
// Derivato da roleContexts (roles[].context): se l'utente ha un ruolo con un dato context,
// quel context è accessibile. I ruoli agnostic espongono tutti i contesti.
func (a *ConfigAuthorizer) GetContexts(roles []string) []*Context {
	hasAgnostic := false
	for _, role := range roles {
		if _, ok := a.agnosticRoles[role]; ok {
			hasAgnostic = true
			break
		}
	}

	seen := make(map[string]struct{})
	var out []*Context

	if hasAgnostic {
		// ruolo agnostic → tutti i contesti disponibili
		for ctxId, label := range a.contextLabels {
			if _, done := seen[ctxId]; !done {
				seen[ctxId] = struct{}{}
				out = append(out, &Context{ID: ctxId, Label: label})
			}
		}
		return out
	}

	for _, role := range roles {
		if ctxId, ok := a.roleContexts[role]; ok && ctxId != "" {
			if _, done := seen[ctxId]; !done {
				seen[ctxId] = struct{}{}
				out = append(out, &Context{ID: ctxId, Label: a.contextLabels[ctxId]})
			}
		}
	}
	return out
}

// GetApps restituisce le app navigabili per i ruoli e il contesto indicati.
//
// Home: il contesto dichiara la sua home via contextHomeApps (da ContextRuleConfig.HomeApp).
// Più contesti possono puntare alla stessa app home.
// In no-context mode (contextID == "") viene inclusa la prima app con isHome e senza home dedicata.
//
// App non-home: accessibili se l'utente ha almeno una capability "ui" per quell'app-id.
func (a *ConfigAuthorizer) GetApps(roles []string, contextID string) []*App {
	accessibleApps := make(map[string]struct{})
	for _, role := range roles {
		for _, cid := range a.roleCaps[role] {
			if cap, ok := a.caps[cid]; ok && cap.category == "ui" && cap.appID != "" {
				accessibleApps[cap.appID] = struct{}{}
			}
		}
	}

	cidLower := strings.ToLower(contextID)
	homeAppID := a.contextHomeApps[contextID]
	var out []*App

	for _, app := range a.apps {
		if app.isHome {
			if contextID != "" {
				// includi solo la home designata per questo contesto
				if app.id == homeAppID {
					out = append(out, &App{ID: app.id, Description: app.description, Path: "/" + cidLower + "/", Icon: app.icon, Order: app.order, ContextID: contextID})
				}
			} else {
				// no-context mode: includi le home non designate ad alcun contesto specifico
				isDesignated := false
				for _, hid := range a.contextHomeApps {
					if hid == app.id {
						isDesignated = true
						break
					}
				}
				if !isDesignated {
					out = append(out, &App{ID: app.id, Description: app.description, Path: "/", Icon: app.icon, Order: app.order})
				}
			}
			continue
		}
		if _, ok := accessibleApps[app.id]; !ok {
			continue
		}
		path, appCtxID := app.basePath, ""
		if contextID != "" {
			path, appCtxID = "/"+cidLower+app.basePath, contextID
		}
		out = append(out, &App{ID: app.id, Description: app.description, Path: path, Icon: app.icon, Order: app.order, ContextID: appCtxID})
	}
	return out
}

// Match verifica se un ruolo ha la capability "api" con l'operationId dato (uso backend).
func (a *ConfigAuthorizer) Match(roles []string, operationId string) bool {
	for _, r := range roles {
		for _, cid := range a.roleCaps[r] {
			if cap, ok := a.caps[cid]; ok && cap.category == "api" && cap.operationID == operationId {
				return true
			}
		}
	}
	return false
}

// MatchRequest verifica l'autorizzazione su path+method tramite le capability "api".
// Allineato ad AuthorizationLut: un ruolo è autorizzato se possiede almeno una capability
// "api" il cui path+methods corrisponde alla richiesta.
func (a *ConfigAuthorizer) MatchRequest(roles []string, path, method string) bool {
	for _, role := range roles {
		for _, cid := range a.roleCaps[role] {
			cap, ok := a.caps[cid]
			if !ok || cap.category != "api" || cap.apiPath == "" {
				continue
			}
			if matchGlob(cap.apiPath, path) && matchMethods(cap.apiMethods, method) {
				return true
			}
		}
	}
	return false
}

// GetServerCapabilities restituisce gli ID delle capability "api" per i ruoli.
// Iniettato dal gateway come X-Capabilities nelle richieste proxiate al backend.
func (a *ConfigAuthorizer) GetServerCapabilities(roles []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, r := range roles {
		for _, cid := range a.roleCaps[r] {
			if _, ok := seen[cid]; ok {
				continue
			}
			if cap, ok := a.caps[cid]; ok && cap.category == "api" {
				seen[cid] = struct{}{}
				out = append(out, cid)
			}
		}
	}
	return out
}

// GetCapabilities restituisce gli ID delle capability "action_ui" per i ruoli.
// Filtrate per appId (match esatto o capability senza appId).
func (a *ConfigAuthorizer) GetCapabilities(roles []string, appId string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, r := range roles {
		for _, cid := range a.roleCaps[r] {
			if _, ok := seen[cid]; ok {
				continue
			}
			cap, ok := a.caps[cid]
			if !ok || cap.category != "action_ui" {
				continue
			}
			if cap.appID == "" || cap.appID == appId {
				seen[cid] = struct{}{}
				out = append(out, cid)
			}
		}
	}
	return out
}

// GetPaths restituisce le voci di menu (categoria "ui") per i ruoli, filtrate per appId.
func (a *ConfigAuthorizer) GetPaths(roles []string, appId string) []*Path {
	seen := make(map[string]struct{})
	var out []*Path
	for _, r := range roles {
		for _, cid := range a.roleCaps[r] {
			if _, ok := seen[cid]; ok {
				continue
			}
			cap, ok := a.caps[cid]
			if !ok || cap.category != "ui" || cap.appID != appId {
				continue
			}
			seen[cid] = struct{}{}
			out = append(out, &Path{ID: cap.id, Description: cap.description, Icon: cap.icon, Order: cap.order, Endpoint: cap.endpoint, Menu: cap.isMenu})
		}
	}
	return out
}

// HasCapability verifica se un ruolo possiede la capability "action_api" indicata.
func (a *ConfigAuthorizer) HasCapability(roles []string, capabilityId string) bool {
	for _, r := range roles {
		for _, cid := range a.roleCaps[r] {
			if cid == capabilityId {
				if cap, ok := a.caps[cid]; ok && cap.category == "action_api" {
					return true
				}
			}
		}
	}
	return false
}

func matchGlob(pattern, path string) bool {
	if pattern == path {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return path == prefix || strings.HasPrefix(path, prefix+"/")
	}
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	rp := strings.Split(strings.Trim(path, "/"), "/")
	if len(pp) != len(rp) {
		return false
	}
	for i := range pp {
		seg := pp[i]
		if seg == "*" || strings.HasPrefix(seg, ":") || (strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")) {
			continue
		}
		if seg != rp[i] {
			return false
		}
	}
	return true
}

func matchMethods(methods []string, method string) bool {
	if len(methods) == 0 {
		return true
	}
	for _, m := range methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}
