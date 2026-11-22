// Package i18n holds Niro's message catalogs. Every user-facing string the
// bot sends goes through a Key; English is the source language and other
// languages are added by dropping a locales/<lang>.toml file next to en.toml.
package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localeFS embed.FS

// Key identifies a message. Keys are declared with define so the catalog
// test can verify that English has every one of them.
type Key string

var (
	definedMu sync.Mutex
	defined   []Key
)

func define(id string) Key {
	definedMu.Lock()
	defer definedMu.Unlock()
	defined = append(defined, Key(id))
	return Key(id)
}

// Defined returns every key declared in code.
func Defined() []Key {
	definedMu.Lock()
	defer definedMu.Unlock()
	return append([]Key(nil), defined...)
}

// Catalog resolves keys to text in a language, falling back to English.
type Catalog struct {
	bundle *goi18n.Bundle
	langs  []language.Tag
	ids    map[language.Tag]map[string]bool
}

// Load reads the embedded catalogs.
func Load() (*Catalog, error) { return LoadFS(localeFS, "locales") }

// LoadFS reads catalogs from fsys/dir (one <lang>.toml per language).
func LoadFS(fsys fs.FS, dir string) (*Catalog, error) {
	b := goi18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading locales: %w", err)
	}
	c := &Catalog{bundle: b, ids: map[language.Tag]map[string]bool{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		mf, err := b.LoadMessageFileFS(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", e.Name(), err)
		}
		ids := make(map[string]bool, len(mf.Messages))
		for _, m := range mf.Messages {
			ids[m.ID] = true
		}
		c.ids[mf.Tag] = ids
		c.langs = append(c.langs, mf.Tag)
	}
	if c.ids[language.English] == nil {
		return nil, fmt.Errorf("locales: en.toml is required")
	}
	sort.Slice(c.langs, func(i, j int) bool { return c.langs[i].String() < c.langs[j].String() })
	return c, nil
}

// Languages returns the languages that have a catalog.
func (c *Catalog) Languages() []language.Tag { return append([]language.Tag(nil), c.langs...) }

// T translates key for the given locale (a BCP 47 tag such as "en-US" or
// "nl"); unknown or untranslated messages fall back to English. data fills
// {{.Placeholders}}. A key missing even in English renders as the key itself
// so the bug is visible instead of an empty message.
func (c *Catalog) T(locale string, key Key, data map[string]any) string {
	loc := goi18n.NewLocalizer(c.bundle, locale, "en")
	s, err := loc.Localize(&goi18n.LocalizeConfig{MessageID: string(key), TemplateData: data})
	if err != nil && s == "" {
		return string(key)
	}
	return s
}

// Translations returns the text of key in every non-English language that
// translates it (used for Discord's command name/description localizations).
func (c *Catalog) Translations(key Key) map[language.Tag]string {
	out := map[language.Tag]string{}
	for _, tag := range c.langs {
		if tag == language.English || !c.ids[tag][string(key)] {
			continue
		}
		out[tag] = c.T(tag.String(), key, nil)
	}
	return out
}

// Missing lists the defined keys that a language does not translate.
func (c *Catalog) Missing(tag language.Tag) []Key {
	var out []Key
	for _, k := range Defined() {
		if !c.ids[tag][string(k)] {
			out = append(out, k)
		}
	}
	return out
}
