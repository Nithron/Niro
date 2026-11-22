package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// ChangeSource records where a configuration change came from (for the audit log).
type ChangeSource string

const (
	SourceDashboard ChangeSource = "dashboard"
	SourceCommand   ChangeSource = "command"
	SourceSystem    ChangeSource = "system"
)

var (
	ErrUnknownModule   = errors.New("unknown module")
	ErrModuleAlwaysOn  = errors.New("module cannot be disabled")
	ErrUnknownGuild    = errors.New("guild not known to Nithro")
	errNilDependencies = errors.New("config service: nil dependency")
)

// StoredModuleState is a module row as persisted for one guild.
type StoredModuleState struct {
	Module    ModuleID
	Enabled   bool
	Version   int64
	UpdatedAt time.Time
}

// ModuleState is the effective state of a module in a guild: the stored row
// if one exists, otherwise the module's default.
type ModuleState struct {
	Module    Module
	Enabled   bool
	Version   int64 // 0 means "never changed, default applies"
	UpdatedAt time.Time
}

// ConfigChange is broadcast to every process after a guild's config changed.
type ConfigChange struct {
	GuildID snowflake.ID `json:"guild_id,string"`
	Module  ModuleID     `json:"module"`
	Version int64        `json:"version"`
}

// ConfigStore persists module state. SetModuleEnabled must write the audit
// entry in the same transaction and return ErrUnknownGuild if the guild row
// does not exist.
type ConfigStore interface {
	ListModuleStates(ctx context.Context, guildID snowflake.ID) ([]StoredModuleState, error)
	SetModuleEnabled(ctx context.Context, guildID snowflake.ID, module ModuleID, enabled bool, actorID *snowflake.ID, source ChangeSource) (StoredModuleState, error)
}

// ChangeBus fans config changes out to all processes (bot shards, API replicas).
type ChangeBus interface {
	PublishConfigChange(ctx context.Context, change ConfigChange) error
	// SubscribeConfigChanges blocks, calling fn for each change, until ctx is
	// done or the subscription fails.
	SubscribeConfigChanges(ctx context.Context, fn func(ConfigChange)) error
}

// ConfigService is the single entry point for reading and changing per-guild
// module configuration. Reads are served from a local cache that is
// invalidated by bus messages, with a TTL as a safety net.
type ConfigService struct {
	registry *Registry
	store    ConfigStore
	bus      ChangeBus
	log      *slog.Logger
	ttl      time.Duration
	now      func() time.Time

	mu    sync.Mutex
	cache map[snowflake.ID]cachedStates
}

type cachedStates struct {
	states map[ModuleID]StoredModuleState
	at     time.Time
}

func NewConfigService(registry *Registry, store ConfigStore, bus ChangeBus, log *slog.Logger) (*ConfigService, error) {
	if registry == nil || store == nil || bus == nil || log == nil {
		return nil, errNilDependencies
	}
	return &ConfigService{
		registry: registry,
		store:    store,
		bus:      bus,
		log:      log,
		ttl:      5 * time.Minute,
		now:      time.Now,
		cache:    make(map[snowflake.ID]cachedStates),
	}, nil
}

// Registry returns the module registry the service validates against.
func (s *ConfigService) Registry() *Registry { return s.registry }

// ModuleStates returns the effective state of every registered module.
func (s *ConfigService) ModuleStates(ctx context.Context, guildID snowflake.ID) ([]ModuleState, error) {
	stored, err := s.load(ctx, guildID)
	if err != nil {
		return nil, err
	}
	out := make([]ModuleState, 0, len(s.registry.modules))
	for _, m := range s.registry.modules {
		out = append(out, effective(m, stored))
	}
	return out, nil
}

// IsEnabled reports whether a module is enabled for a guild.
func (s *ConfigService) IsEnabled(ctx context.Context, guildID snowflake.ID, id ModuleID) (bool, error) {
	m, ok := s.registry.Get(id)
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrUnknownModule, id)
	}
	if m.AlwaysOn {
		return true, nil
	}
	stored, err := s.load(ctx, guildID)
	if err != nil {
		return false, err
	}
	return effective(m, stored).Enabled, nil
}

// SetEnabled changes a module's state, records who did it, and tells every
// process to drop its cached copy.
func (s *ConfigService) SetEnabled(ctx context.Context, guildID snowflake.ID, id ModuleID, enabled bool, actorID *snowflake.ID, source ChangeSource) (ModuleState, error) {
	m, ok := s.registry.Get(id)
	if !ok {
		return ModuleState{}, fmt.Errorf("%w: %q", ErrUnknownModule, id)
	}
	if m.AlwaysOn && !enabled {
		return ModuleState{}, fmt.Errorf("%w: %q", ErrModuleAlwaysOn, id)
	}
	row, err := s.store.SetModuleEnabled(ctx, guildID, id, enabled, actorID, source)
	if err != nil {
		return ModuleState{}, err
	}
	s.Invalidate(guildID)
	change := ConfigChange{GuildID: guildID, Module: id, Version: row.Version}
	if err := s.bus.PublishConfigChange(ctx, change); err != nil {
		// The write succeeded; other processes will catch up when their cache TTL expires.
		s.log.WarnContext(ctx, "publishing config change failed", "guild_id", guildID, "module", id, "err", err)
	}
	return ModuleState{Module: m, Enabled: row.Enabled, Version: row.Version, UpdatedAt: row.UpdatedAt}, nil
}

// Invalidate drops the cached state of one guild.
func (s *ConfigService) Invalidate(guildID snowflake.ID) {
	s.mu.Lock()
	delete(s.cache, guildID)
	s.mu.Unlock()
}

// RunInvalidation listens for config changes from other processes and drops
// the affected guild from the local cache. It resubscribes with backoff until
// ctx is done. onChange (optional) is called after each invalidation.
func (s *ConfigService) RunInvalidation(ctx context.Context, onChange func(ConfigChange)) {
	backoff := time.Second
	for ctx.Err() == nil {
		err := s.bus.SubscribeConfigChanges(ctx, func(c ConfigChange) {
			s.Invalidate(c.GuildID)
			if onChange != nil {
				onChange(c)
			}
		})
		if ctx.Err() != nil {
			return
		}
		// We may have missed messages while disconnected: drop everything.
		s.mu.Lock()
		clear(s.cache)
		s.mu.Unlock()
		s.log.WarnContext(ctx, "config change subscription ended, retrying", "err", err, "backoff", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (s *ConfigService) load(ctx context.Context, guildID snowflake.ID) (map[ModuleID]StoredModuleState, error) {
	s.mu.Lock()
	c, ok := s.cache[guildID]
	s.mu.Unlock()
	if ok && s.now().Sub(c.at) < s.ttl {
		return c.states, nil
	}
	rows, err := s.store.ListModuleStates(ctx, guildID)
	if err != nil {
		return nil, fmt.Errorf("loading module states: %w", err)
	}
	states := make(map[ModuleID]StoredModuleState, len(rows))
	for _, r := range rows {
		states[r.Module] = r
	}
	s.mu.Lock()
	s.cache[guildID] = cachedStates{states: states, at: s.now()}
	s.mu.Unlock()
	return states, nil
}

func effective(m Module, stored map[ModuleID]StoredModuleState) ModuleState {
	st := ModuleState{Module: m, Enabled: m.DefaultEnabled}
	if row, ok := stored[m.ID]; ok {
		st.Enabled, st.Version, st.UpdatedAt = row.Enabled, row.Version, row.UpdatedAt
	}
	if m.AlwaysOn {
		st.Enabled = true
	}
	return st
}
