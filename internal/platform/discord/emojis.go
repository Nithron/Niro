package discord

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/assets"
)

// Brand emoji names (files in assets/brand/png/discord/emojis).
const (
	EmojiNiro   = "niro"
	EmojiOK     = "ok"
	EmojiWarn   = "warn"
	EmojiError  = "error"
	EmojiInfo   = "info"
	EmojiShield = "shield"
	EmojiLock   = "lock"
)

// fallbacks are used until (or if) the application emojis are uploaded.
var fallbacks = map[string]string{
	EmojiNiro: "", EmojiOK: "✅", EmojiWarn: "⚠️", EmojiError: "❌",
	EmojiInfo: "ℹ️", EmojiShield: "🛡️", EmojiLock: "🔒",
}

// Emojis uploads Niro's brand emojis to the Discord application (they are
// owned by the app and usable in every server) and formats them in messages.
type Emojis struct {
	rest  rest.Applications
	appID snowflake.ID
	log   *slog.Logger

	mu  sync.RWMutex
	ids map[string]snowflake.ID
}

func NewEmojis(r rest.Applications, appID snowflake.ID, log *slog.Logger) *Emojis {
	return &Emojis{rest: r, appID: appID, log: log, ids: map[string]snowflake.ID{}}
}

// Sync uploads any brand emoji the application does not have yet. Existing
// emojis are left untouched (delete one in the Developer Portal to re-upload).
func (e *Emojis) Sync(ctx context.Context) error {
	existing, err := e.rest.GetApplicationEmojis(e.appID, rest.WithCtx(ctx))
	if err != nil {
		return fmt.Errorf("listing application emojis: %w", err)
	}
	have := make(map[string]snowflake.ID, len(existing))
	for _, em := range existing {
		have[em.Name] = em.ID
	}

	files, err := fs.ReadDir(assets.Emojis, assets.EmojiDir)
	if err != nil {
		return err
	}
	var uploaded []string
	for _, f := range files {
		name := strings.TrimSuffix(f.Name(), ".png")
		if _, ok := have[name]; ok {
			continue
		}
		data, err := fs.ReadFile(assets.Emojis, path.Join(assets.EmojiDir, f.Name()))
		if err != nil {
			return err
		}
		em, err := e.rest.CreateApplicationEmoji(e.appID, discord.EmojiCreate{
			Name: name, Image: *discord.NewIconRaw(discord.IconTypePNG, data),
		}, rest.WithCtx(ctx))
		if err != nil {
			return fmt.Errorf("uploading emoji %q: %w", name, err)
		}
		have[name] = em.ID
		uploaded = append(uploaded, name)
	}

	e.mu.Lock()
	e.ids = have
	e.mu.Unlock()
	if len(uploaded) > 0 {
		e.log.Info("uploaded application emojis", "names", uploaded)
	}
	return nil
}

// Get returns the message markup for a brand emoji, falling back to Unicode.
func (e *Emojis) Get(name string) string {
	e.mu.RLock()
	id, ok := e.ids[name]
	e.mu.RUnlock()
	if ok {
		return fmt.Sprintf("<:%s:%s>", name, id)
	}
	return fallbacks[name]
}
