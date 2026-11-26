package moderation

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var errDurationSyntax = errors.New("invalid duration")

// maxParsedDuration bounds what ParseDuration accepts (about 100 years).
const maxParsedDuration = 100 * 365 * 24 * time.Hour

// ParseDuration parses moderator-friendly durations: a sequence of
// number+unit pairs with units s, m, h, d, w (e.g. "10m", "1d12h", "2w").
// Spaces are ignored and units are case-insensitive.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if s == "" {
		return 0, errDurationSyntax
	}
	var total time.Duration
	for s != "" {
		i := 0
		for i < len(s) && unicode.IsDigit(rune(s[i])) {
			i++
		}
		if i == 0 || i == len(s) {
			return 0, errDurationSyntax
		}
		n, err := strconv.Atoi(s[:i])
		if err != nil || n > 100000 {
			return 0, errDurationSyntax
		}
		unit := s[i]
		var d time.Duration
		switch unit {
		case 's':
			d = time.Second
		case 'm':
			d = time.Minute
		case 'h':
			d = time.Hour
		case 'd':
			d = 24 * time.Hour
		case 'w':
			d = 7 * 24 * time.Hour
		default:
			return 0, errDurationSyntax
		}
		// Guard against overflow: "100000w" would wrap around int64.
		if time.Duration(n) > (maxParsedDuration-total)/d {
			return 0, errDurationSyntax
		}
		total += time.Duration(n) * d
		s = s[i+1:]
	}
	return total, nil
}

// FormatDuration renders a duration compactly, largest units first, e.g.
// "1d 12h" or "45m". Seconds are only shown for durations under a minute.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	var parts []string
	for _, u := range []struct {
		d    time.Duration
		name string
	}{{7 * 24 * time.Hour, "w"}, {24 * time.Hour, "d"}, {time.Hour, "h"}, {time.Minute, "m"}} {
		if n := d / u.d; n > 0 {
			parts = append(parts, fmt.Sprintf("%d%s", n, u.name))
			d -= n * u.d
		}
	}
	return strings.Join(parts, " ")
}
