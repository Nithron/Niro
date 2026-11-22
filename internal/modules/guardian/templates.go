package guardian

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// template is a known scam message pattern. Patterns run on folded text
// (lower case, homoglyphs and digit swaps undone), so "FR3E N1TRO" matches.
type template struct {
	id string
	re *regexp.Regexp
}

var templates = []template{
	{"free_nitro", regexp.MustCompile(`(free|gift(ed|ing)?|giv(e|ing) ?away|claim|drop).{0,40}nitro|nitro.{0,40}(free|gift|giv(e|ing) ?away|claim|for everyone)`)},
	{"steam_gift", regexp.MustCompile(`steam.{0,30}(gift|giv(e|ing) ?away|free|\$ ?\d+)|(gift|\$ ?\d+).{0,30}steam`)},
	{"false_report", regexp.MustCompile(`(accident(al)?ly|mistaken(ly)?|by mistake).{0,40}report(ed)?|report(ed)? (you|your account).{0,40}(accident|mistake)`)},
	{"crypto_airdrop", regexp.MustCompile(`airdrop|free (crypto|btc|bitcoin|eth|usdt|sol)|claim (your )?(tokens?|rewards?|nfts?)|(double|2x) your (crypto|btc|eth)`)},
	{"adult_bait", regexp.MustCompile(`(teen|leak(ed|s)?|nudes|onlyfans|18\+).{0,30}(free|join|link|discord\.gg)`)},
	{"mass_ping_link", regexp.MustCompile(`@(everyone|here)`)}, // only counts together with a link
}

// fold prepares text for template matching.
func fold(s string) string {
	s = norm.NFKC.String(strings.ToLower(s))
	s = templateConfusables.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// templateConfusables is like the domain skeleton but keeps "l" and "-" so
// ordinary words still read normally.
var templateConfusables = strings.NewReplacer(
	"а", "a", "с", "c", "е", "e", "і", "i", "о", "o", "р", "p", "ѕ", "s", "х", "x", "у", "y", "т", "t", "к", "k", "м", "m",
	"α", "a", "ε", "e", "ι", "i", "ο", "o", "ρ", "p", "τ", "t",
	"0", "o", "1", "i", "3", "e", "4", "a", "7", "t", "ı", "i",
)

// matchTemplates returns the IDs of the templates the message matches.
// "$50" survives folding because digits after "$" are matched before folding.
func matchTemplates(content string, hasLink bool) []string {
	if content == "" {
		return nil
	}
	lower := strings.ToLower(content)
	folded := fold(content)
	var out []string
	for _, t := range templates {
		if t.id == "mass_ping_link" && !hasLink {
			continue
		}
		if t.re.MatchString(folded) || t.re.MatchString(lower) {
			out = append(out, t.id)
		}
	}
	return out
}
