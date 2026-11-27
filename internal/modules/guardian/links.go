package guardian

import (
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
)

// Link is a URL found in a message. Niro never fetches it: every check works
// on the text of the URL alone (privacy and SSRF safety, plan §7 M2).
type Link struct {
	URL string
	// Host is lower-case, without "www." and trailing dots, decoded from
	// punycode to Unicode so look-alike checks see what the user sees.
	Host string
	// Domain is the registrable domain (eTLD+1), e.g. "evil.co.uk".
	Domain string
	// Text is the visible text of a masked markdown link ([text](url)).
	Text   string
	Masked bool
}

var (
	maskedLinkRe = regexp.MustCompile(`\[([^\]\n]{1,256})\]\(\s*<?(https?://[^\s)>]+)>?(?:\s+"[^"]*")?\s*\)`)
	bareLinkRe   = regexp.MustCompile(`(?i)https?://[^\s<>()\[\]"'` + "`" + `]+`)
	// domainLikeRe finds something that looks like a domain in link text.
	domainLikeRe = regexp.MustCompile(`(?i)(?:https?://)?((?:[\p{L}\p{N}-]+\.)+[\p{L}]{2,})`)
)

// MaxLinksPerMessage bounds the work done on one message.
const MaxLinksPerMessage = 20

// ExtractLinks returns the http(s) links in a message, masked links first.
func ExtractLinks(content string) []Link {
	var out []Link
	seen := map[string]bool{}
	add := func(l Link) {
		if len(out) >= MaxLinksPerMessage {
			return
		}
		key := l.URL + "\x00" + l.Text
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, l)
	}

	rest := content
	for _, m := range maskedLinkRe.FindAllStringSubmatchIndex(content, -1) {
		text, raw := content[m[2]:m[3]], content[m[4]:m[5]]
		if l, ok := parseLink(raw); ok {
			l.Text, l.Masked = text, true
			add(l)
		}
		// Blank out the masked link so the bare-link pass does not see the URL twice.
		rest = rest[:m[0]] + strings.Repeat(" ", m[1]-m[0]) + rest[m[1]:]
	}
	for _, raw := range bareLinkRe.FindAllString(rest, -1) {
		raw = strings.TrimRight(raw, ".,;:!?*_~|>")
		if l, ok := parseLink(raw); ok {
			add(l)
		}
	}
	return out
}

func parseLink(raw string) (Link, bool) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return Link{}, false
	}
	host, ok := NormalizeHost(u.Hostname())
	if !ok {
		return Link{}, false
	}
	return Link{URL: raw, Host: host, Domain: RegistrableDomain(host)}, true
}

var idnaProfile = idna.New(idna.MapForLookup(), idna.Transitional(false))

// NormalizeHost lower-cases a host, strips "www." and trailing dots, and
// decodes punycode. It rejects IP literals and malformed names.
func NormalizeHost(h string) (string, bool) {
	h = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
	if h == "" || strings.ContainsAny(h, ":[]/ @") || !strings.Contains(h, ".") ||
		strings.ContainsFunc(h, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", false
	}
	if uh, err := idnaProfile.ToUnicode(h); err == nil {
		h = uh
	} else if strings.Contains(h, "xn--") {
		return "", false
	}
	// IDNA mapping turns fullwidth "／", "＠", "：" and "。" into their ASCII
	// forms, so check again after it.
	h = strings.TrimSuffix(h, ".")
	if strings.ContainsAny(h, ":[]/ @") ||
		strings.ContainsFunc(h, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", false
	}
	// Strip "www." (repeatedly) while a registrable domain is left behind,
	// so "www.co.uk" stays as it is instead of becoming a bare suffix.
	for strings.HasPrefix(h, "www.") {
		if _, err := publicsuffix.EffectiveTLDPlusOne(h[4:]); err != nil {
			break
		}
		h = h[4:]
	}
	if len(h) > 253 || isIPv4(h) || !strings.Contains(h, ".") {
		return "", false
	}
	for _, label := range strings.Split(h, ".") {
		if label == "" || len(label) > 63 {
			return "", false
		}
	}
	return h, true
}

func isIPv4(h string) bool {
	parts := strings.Split(h, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || strings.Trim(p, "0123456789") != "" {
			return false
		}
	}
	return true
}

// RegistrableDomain returns eTLD+1 ("a.b.example.co.uk" → "example.co.uk"),
// or the host itself if it has no public suffix.
func RegistrableDomain(host string) string {
	d, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return d
}

// parentDomains lists host and each parent down to the registrable domain:
// "a.b.evil.ru" → [a.b.evil.ru b.evil.ru evil.ru].
func parentDomains(host string) []string {
	reg := RegistrableDomain(host)
	out := []string{host}
	for h := host; h != reg; {
		i := strings.IndexByte(h, '.')
		if i < 0 {
			break
		}
		h = h[i+1:]
		out = append(out, h)
	}
	return out
}

// maskedMismatch reports whether a masked link's visible text names a
// different site than the one it opens, e.g. [discord.com/gift](https://evil.ru).
func maskedMismatch(l Link) (shown string, ok bool) {
	if !l.Masked {
		return "", false
	}
	for _, m := range domainLikeRe.FindAllStringSubmatch(l.Text, 3) {
		h, valid := NormalizeHost(m[1])
		if !valid {
			continue
		}
		// Only real-looking domains count ("v1.2" or "file.txt" do not).
		if _, icann := publicsuffix.PublicSuffix(h); !icann {
			continue
		}
		if RegistrableDomain(h) != l.Domain {
			return h, true
		}
	}
	return "", false
}

// uniqueDomains returns the sorted registrable domains of links.
func uniqueDomains(links []Link) []string {
	var out []string
	for _, l := range links {
		if !slices.Contains(out, l.Domain) {
			out = append(out, l.Domain)
		}
	}
	slices.Sort(out)
	return out
}
