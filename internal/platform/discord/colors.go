package discord

// Colour bars in Discord take a role, never a colour per category or per
// moderation action (DESIGN-APPS.md in nithron-site, "A face inside another
// product"). Discord shows one value on its dark and its light theme, so each
// role has one value that holds 3:1 on both (colors_test.go). Each is mixed in
// OKLab from the role's own pair in the Nithron tokens (line.css 0.4.0: light
// and dark --status-*-bar), at the point that reads best on Discord's four
// grounds. Neutral replies, panels, case cards and log entries have no bar.
const (
	barNone    = 0        // no bar (Discord's accent_color 0)
	barPass    = 0x21974B // done: --status-pass #017b37 to #54dd7d
	barEarlier = 0xAA7C1D // waiting, a warning, a lockdown: --status-earlier #916304 to #f0c551
	barFailed  = 0xF73937 // an error: --status-failed #d40c1a to #ff413d
)
