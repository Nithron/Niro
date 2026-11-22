package guardian

import (
	"slices"
	"strings"
	"sync/atomic"

	"golang.org/x/text/unicode/norm"
)

// DomainSet is the in-memory scam-domain blocklist. It is swapped atomically
// when the feed updates, so lookups on the message path never lock.
type DomainSet struct {
	m atomic.Pointer[map[string]struct{}]
}

func NewDomainSet(domains ...string) *DomainSet {
	s := &DomainSet{}
	s.Replace(domains)
	return s
}

// Replace swaps in a new list. Entries are normalized; invalid ones are dropped.
func (s *DomainSet) Replace(domains []string) {
	m := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		if h, ok := NormalizeHost(d); ok {
			m[h] = struct{}{}
		}
	}
	s.m.Store(&m)
}

func (s *DomainSet) Len() int {
	if p := s.m.Load(); p != nil {
		return len(*p)
	}
	return 0
}

// Match returns the listed entry that covers host (the host itself or one of
// its parents), or "".
func (s *DomainSet) Match(host string) string {
	p := s.m.Load()
	if p == nil {
		return ""
	}
	for _, h := range parentDomains(host) {
		if _, ok := (*p)[h]; ok {
			return h
		}
	}
	return ""
}

// matchList is Match for a small per-server list.
func matchList(list []string, host string) string {
	for _, h := range parentDomains(host) {
		if slices.Contains(list, h) {
			return h
		}
	}
	return ""
}

// brand is a frequently impersonated service and the domains it really owns.
type brand struct {
	name     string   // skeleton of the brand's main label
	official []string // registrable domains owned by the brand
}

var brands = []brand{
	{"discord", []string{
		"discord.com", "discord.gg", "discordapp.com", "discordapp.net", "discord.media", "discord.gift",
		"discord.new", "discord.dev", "discord.co", "discordstatus.com", "dis.gd", "discord.design",
		"discord.store", "discord.tools", "discordmerch.com", "discord.gifts",
	}},
	{"discordapp", []string{"discordapp.com", "discordapp.net"}},
	{"steamcommunity", []string{"steamcommunity.com"}},
	{"steampowered", []string{"steampowered.com"}},
	{"roblox", []string{"roblox.com", "rbxcdn.com"}},
	{"epicgames", []string{"epicgames.com"}},
	{"twitch", []string{"twitch.tv"}},
	{"paypal", []string{"paypal.com", "paypal.me"}},
}

// scamWords combined with a brand word in the registrable label mark a scam
// domain ("discord-nitro.gift", "free-nitro.xyz"). Only Discord words are
// used: "steam" + "gift" would hit real sites such as steamgifts.com.
var (
	scamWords      = []string{"nitro", "gift", "free", "claim", "promo", "airdrop", "giveaway", "drop", "reward", "bonus"}
	scamBrandWords = []string{"discord", "nitro"}
)

// knownGood are real sites whose names would otherwise read as a brand
// (usually because they sit on a public suffix such as js.org).
var knownGood = []string{"discord.js.org"}

func isOfficial(domain string) bool {
	if slices.Contains(knownGood, domain) {
		return true
	}
	for _, b := range brands {
		if slices.Contains(b.official, domain) {
			return true
		}
	}
	return false
}

// Lookalike checks whether host impersonates a known brand. It returns the
// brand imitated, or "". Real sites that merely mention a brand
// (discords.com, discord.mycommunity.org) must not match.
func Lookalike(host string) string {
	domain := RegistrableDomain(host)
	if isOfficial(domain) {
		return ""
	}
	// 1. An official domain used as a prefix: discord.com.evil.ru.
	for _, b := range brands {
		for _, o := range b.official {
			if strings.HasPrefix(host, o+".") {
				return b.name
			}
		}
	}
	// 2. The registrable label reads as a brand once homoglyphs and
	//    look-alike letters are folded (dlscord.com, dіscord.gg with a
	//    Cyrillic і), or is one swapped/substituted letter away (discrod.gift).
	label := skeleton(strings.SplitN(domain, ".", 2)[0])
	for _, b := range brands {
		if label == b.name {
			return b.name
		}
		if len(b.name) >= 6 && len(label) == len(b.name) && damerau(label, b.name) == 1 {
			return b.name
		}
	}
	// 3. Brand word + scam word: discord-nitro.gift, free-nitro.xyz.
	for _, bw := range scamBrandWords {
		if !strings.Contains(label, bw) {
			continue
		}
		for _, sw := range scamWords {
			if sw != bw && strings.Contains(label, sw) {
				return bw
			}
		}
	}
	return ""
}

// confusables folds characters that look like Latin letters into them.
// Covers the Cyrillic and Greek homoglyphs and digit swaps seen in scam domains.
var confusables = strings.NewReplacer(
	// Cyrillic
	"а", "a", "в", "b", "с", "c", "ԁ", "d", "е", "e", "һ", "h", "і", "i", "ј", "j", "к", "k",
	"ӏ", "l", "м", "m", "о", "o", "р", "p", "ԛ", "q", "г", "r", "ѕ", "s", "т", "t", "υ", "u",
	"ԝ", "w", "х", "x", "у", "y", "ь", "b",
	// Greek
	"α", "a", "β", "b", "ε", "e", "η", "n", "ι", "i", "κ", "k", "ν", "v", "ο", "o", "ρ", "p",
	"τ", "t", "χ", "x", "γ", "y",
	// Latin look-alikes and digits
	"ı", "i", "ł", "l", "ø", "o", "0", "o", "1", "l", "3", "e", "4", "a", "5", "s", "7", "t",
	"rn", "m", "vv", "w", "-", "", "_", "",
)

// skeleton maps a label to a canonical form for look-alike comparison.
// "l" and "i" are treated as the same letter (dlscord ≈ discord).
func skeleton(s string) string {
	s = norm.NFKD.String(strings.ToLower(s))
	// Drop combining marks (diacritics): dìscord → discord.
	s = strings.Map(func(r rune) rune {
		if r >= 0x300 && r <= 0x36f {
			return -1
		}
		return r
	}, s)
	s = confusables.Replace(s)
	return strings.ReplaceAll(s, "l", "i")
}

func init() {
	// Brand names are compared as skeletons too.
	for i := range brands {
		brands[i].name = skeleton(brands[i].name)
	}
	for i := range scamBrandWords {
		scamBrandWords[i] = skeleton(scamBrandWords[i])
	}
	for i := range scamWords {
		scamWords[i] = skeleton(scamWords[i])
	}
}

// damerau is the optimal-string-alignment distance (edits incl. swaps).
func damerau(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if d := len(ra) - len(rb); d > 1 || d < -1 {
		return 2 // only distance ≤ 1 matters to callers
	}
	prev2 := make([]int, len(rb)+1)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(rb)]
}
