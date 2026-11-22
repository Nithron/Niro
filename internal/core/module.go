// Package core holds Nithro's platform-agnostic domain: modules, per-guild
// configuration and the rules that apply regardless of chat platform.
// It must never import a platform SDK (disgo etc.); adapters translate.
package core

import (
	"fmt"
	"slices"
)

// ModuleID identifies a feature module. IDs are stable: they are stored in the
// database and used in API paths and translation keys.
type ModuleID string

const (
	ModuleCore       ModuleID = "core"
	ModuleModeration ModuleID = "moderation"
	ModuleGuardian   ModuleID = "guardian"
	ModuleLogging    ModuleID = "logging"
	ModuleRoles      ModuleID = "roles"
	ModuleWelcome    ModuleID = "welcome"
	ModuleTickets    ModuleID = "tickets"
)

// Module describes a feature module. Its display name and description live in
// the i18n catalogs under "module.<id>.name" and "module.<id>.description".
type Module struct {
	ID ModuleID
	// DefaultEnabled is the state for a guild that never changed the module.
	DefaultEnabled bool
	// AlwaysOn modules cannot be disabled (e.g. core commands).
	AlwaysOn bool
}

// DefaultModules is the v0.1 module set. Feature code for each module lands in
// its milestone; registering them now lets the dashboard and config pipeline
// work end to end from M0.
func DefaultModules() []Module {
	return []Module{
		{ID: ModuleCore, DefaultEnabled: true, AlwaysOn: true},
		{ID: ModuleModeration, DefaultEnabled: true},
		{ID: ModuleGuardian, DefaultEnabled: true},
		{ID: ModuleLogging},
		{ID: ModuleRoles},
		{ID: ModuleWelcome},
		{ID: ModuleTickets},
	}
}

// Registry is the ordered, immutable set of known modules.
type Registry struct {
	modules []Module
	byID    map[ModuleID]Module
}

func NewRegistry(modules ...Module) (*Registry, error) {
	r := &Registry{byID: make(map[ModuleID]Module, len(modules))}
	for _, m := range modules {
		if m.ID == "" {
			return nil, fmt.Errorf("module with empty ID")
		}
		if _, dup := r.byID[m.ID]; dup {
			return nil, fmt.Errorf("duplicate module %q", m.ID)
		}
		if m.AlwaysOn && !m.DefaultEnabled {
			return nil, fmt.Errorf("module %q is always on but not enabled by default", m.ID)
		}
		r.byID[m.ID] = m
		r.modules = append(r.modules, m)
	}
	return r, nil
}

// All returns the modules in registration order.
func (r *Registry) All() []Module { return slices.Clone(r.modules) }

func (r *Registry) Get(id ModuleID) (Module, bool) {
	m, ok := r.byID[id]
	return m, ok
}
