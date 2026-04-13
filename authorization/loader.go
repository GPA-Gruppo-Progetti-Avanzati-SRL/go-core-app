package authorization

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// NewConfigAuthorizerFromFile carica le regole (capabilities/groups/roles) da un file YAML esterno
// e costruisce un ConfigAuthorizer usando apps, contexts e discovered forniti separatamente.
// Apps e contexts provengono dal gateway config (config.yml), non dal file delle regole.
// Discovered sono le capability rilevate al boot da manifest Angular e file backend.
func NewConfigAuthorizerFromFile(apps []AppRuleConfig, contexts []ContextRuleConfig, path string, discovered []CapabilityRuleConfig) (*ConfigAuthorizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("authorization: cannot read rules file %q: %w", path, err)
	}
	var rules AuthRulesConfig
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("authorization: cannot parse rules file %q: %w", path, err)
	}
	return NewConfigAuthorizerFromConfig(apps, contexts, rules, discovered)
}

// NewConfigAuthorizerFromConfig costruisce un ConfigAuthorizer da apps e contexts del gateway
// combinati con le regole ACL (capabilities/groups/roles) e le capability scoperte al boot.
// Discovered ha priorità minore: rules.Capabilities sovrascrivono per ID in caso di conflitto.
// Restituisce errore se la configurazione è semanticamente invalida o vuota.
func NewConfigAuthorizerFromConfig(apps []AppRuleConfig, contexts []ContextRuleConfig, rules AuthRulesConfig, discovered []CapabilityRuleConfig) (*ConfigAuthorizer, error) {
	if err := validateRules(apps, rules); err != nil {
		return nil, fmt.Errorf("authorization: invalid rules config: %w", err)
	}

	// --- contexts ---
	contextLabels := make(map[string]string, len(contexts))
	contextHomeApps := make(map[string]string, len(contexts))
	for _, c := range contexts {
		contextLabels[c.ID] = c.Label
		if c.HomeApp != "" {
			contextHomeApps[strings.ToUpper(c.ID)] = c.HomeApp
		}
	}

	// --- apps ---
	appEntries := make([]appEntry, 0, len(apps))
	for _, a := range apps {
		appEntries = append(appEntries, appEntry{
			id:          a.ID,
			description: a.Description,
			basePath:    a.BasePath,
			icon:        a.Icon,
			order:       a.Order,
			isHome:      a.BasePath == "/",
		})
	}

	// --- capabilities ---
	// Tutte le capability provengono dalla discovery (manifest Angular + file backend).
	caps := make(map[string]capEntry, len(discovered))
	for _, c := range discovered {
		e := capEntry{
			id:          c.ID,
			category:    c.Category,
			description: c.Description,
			icon:        c.Icon,
			order:       c.Order,
			endpoint:    c.Endpoint,
			isMenu:      c.IsMenu,
			appID:       c.AppID,
		}
		if c.API != nil {
			e.operationID = c.API.OperationID
			e.apiPath = c.API.Path
			e.apiMethods = c.API.Methods
		}
		caps[c.ID] = e
	}

	// --- capability groups: groupID → []capID ---
	groupCaps := make(map[string][]string, len(rules.CapabilityGroups))
	for _, g := range rules.CapabilityGroups {
		groupCaps[g.ID] = g.Capabilities
	}

	// --- roles: espandi groups + dirette → []capID per ruolo ---
	roleCaps := make(map[string][]string, len(rules.Roles))
	roleContexts := make(map[string]string, len(rules.Roles))
	agnosticRoles := make(map[string]struct{})
	for _, r := range rules.Roles {
		roleContexts[r.ID] = r.Context
		if r.Context == "" {
			agnosticRoles[r.ID] = struct{}{}
		}
		seen := make(map[string]struct{})
		var expanded []string
		for _, gid := range r.CapabilityGroups {
			for _, cid := range groupCaps[gid] {
				if _, ok := seen[cid]; !ok {
					seen[cid] = struct{}{}
					expanded = append(expanded, cid)
				}
			}
		}
		for _, cid := range r.Capabilities {
			if _, ok := seen[cid]; !ok {
				seen[cid] = struct{}{}
				expanded = append(expanded, cid)
			}
		}
		roleCaps[r.ID] = expanded
	}

	return &ConfigAuthorizer{
		apps:            appEntries,
		caps:            caps,
		roleCaps:        roleCaps,
		roleContexts:    roleContexts,
		contextLabels:   contextLabels,
		contextHomeApps: contextHomeApps,
		agnosticRoles:   agnosticRoles,
	}, nil
}

// validateRules verifica che la configurazione contenga il minimo indispensabile.
func validateRules(apps []AppRuleConfig, rules AuthRulesConfig) error {
	if len(rules.Roles) == 0 {
		return errors.New("at least one role must be defined")
	}
	if len(apps) == 0 {
		return errors.New("at least one app must be defined")
	}
	return nil
}
