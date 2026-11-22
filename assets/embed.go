// Package assets embeds the brand files the binary needs at runtime.
package assets

import "embed"

// Emojis are the application emojis the bot uploads to its Discord
// application on startup (brand/png/discord/emojis/<name>.png).
//
//go:embed brand/png/discord/emojis/*.png
var Emojis embed.FS

// EmojiDir is the directory of Emojis inside the embedded FS.
const EmojiDir = "brand/png/discord/emojis"
