package discord

import (
	"math"
	"testing"

	"github.com/nithron/niro/internal/modules/guardian"
)

func luminance(rgb int) float64 {
	ch := func(v int) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(rgb>>16&0xFF) + 0.7152*ch(rgb>>8&0xFF) + 0.0722*ch(rgb&0xFF)
}

func contrast(a, b int) float64 {
	x, y := luminance(a), luminance(b)
	if x < y {
		x, y = y, x
	}
	return (x + 0.05) / (y + 0.05)
}

// Every bar holds 3:1 (a line, WCAG 1.4.11) on Discord's dark and light
// grounds: the chat and the container inside it.
func TestBarsReadOnBothDiscordThemes(t *testing.T) {
	grounds := map[string]int{"dark chat": 0x313338, "dark container": 0x2B2D31, "light chat": 0xFFFFFF, "light container": 0xF2F3F5}
	bars := map[string]int{"pass": barPass, "earlier": barEarlier, "failed": barFailed}
	for name, bar := range bars {
		for g, ground := range grounds {
			if r := contrast(bar, ground); r < 3 {
				t.Errorf("%s bar #%06X on %s #%06X: %.2f:1, want 3:1", name, bar, g, ground, r)
			}
		}
	}
}

func TestGuardianBars(t *testing.T) {
	cases := []struct {
		ev   guardian.Event
		want int
	}{
		{guardian.Event{Status: guardian.StatusOpen}, barEarlier},
		{guardian.Event{Status: guardian.StatusActive}, barEarlier},
		{guardian.Event{Status: guardian.StatusRestored}, barPass},
		{guardian.Event{Status: guardian.StatusEnded}, barPass},
		{guardian.Event{Status: guardian.StatusBanned}, barNone},
		{guardian.Event{Status: guardian.StatusKept}, barNone},
		{guardian.Event{Response: guardian.ResponseKick}, barNone},
		{guardian.Event{Response: guardian.ResponseDelete}, barNone},
		{guardian.Event{}, barEarlier},
	}
	for _, c := range cases {
		if got := guardianColor(c.ev); got != c.want {
			t.Errorf("status %q response %q: bar #%06X, want #%06X", c.ev.Status, c.ev.Response, got, c.want)
		}
	}
}
