package guardian

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/disgoorg/snowflake/v2"
)

// Message is a chat message as Guardian sees it.
type Message struct {
	GuildID    snowflake.ID
	ChannelID  snowflake.ID
	ID         snowflake.ID
	AuthorID   snowflake.ID
	AuthorName string
	// AuthorBot is true for bots, webhooks and system messages (never checked).
	AuthorBot bool
	// AuthorModerator is true for the owner and members who can manage
	// messages or moderate members (never checked).
	AuthorModerator bool
	AuthorRoleIDs   []snowflake.ID
	// AuthorJoinedAt is when the author joined the server (zero if unknown).
	AuthorJoinedAt time.Time
	Content        string
	// Attachments identify files without reading them: "name:size".
	Attachments []string
	CreatedAt   time.Time
}

// Verdict is the analysis of one message.
type Verdict struct {
	Signals  []Signal
	Score    int
	Response Response
	Kind     Kind
	// Copies are the messages to delete (the message itself and, for a
	// burst, its copies in other channels).
	Copies      []MessageRef
	Channels    []snowflake.ID
	Domains     []string
	Fingerprint string
}

// Score thresholds.
const (
	scoreQuarantine = 3
	scoreDelete     = 2
	scoreAlert      = 1
)

// minTextLength is the shortest normalized text fingerprinted for bursts;
// shorter texts ("hi", "gm") repeat naturally.
const minTextLength = 12

// fingerprints identifies a message's content for cross-channel matching:
// its normalized text, its set of linked domains, and its attachments.
func fingerprints(m Message, links []Link) []string {
	var fps []string
	if t := normalizeText(m.Content); len([]rune(t)) >= minTextLength {
		fps = append(fps, "t:"+hash(t))
	}
	if len(links) > 0 {
		fps = append(fps, "l:"+hash(strings.Join(uniqueDomains(links), " ")))
	}
	if len(m.Attachments) > 0 {
		a := slices.Clone(m.Attachments)
		slices.Sort(a)
		fps = append(fps, "a:"+hash(strings.Join(a, "\x00")))
	}
	return fps
}

func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8])
}

// normalizeText makes near-identical messages equal: case, spacing,
// punctuation, mentions and the variable parts of URLs are ignored.
func normalizeText(s string) string {
	s = fold(s)
	s = bareLinkRe.ReplaceAllStringFunc(s, func(u string) string {
		if l, ok := parseLink(u); ok {
			return " " + l.Domain + " "
		}
		return " "
	})
	var b strings.Builder
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "<@") || strings.HasPrefix(f, "<#") {
			continue // mentions differ per channel
		}
		for _, r := range f {
			if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || r == '$' {
				b.WriteRune(r)
			}
		}
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

// exempt reports whether Guardian ignores the message's author.
func exempt(m Message, s Settings) bool {
	if m.AuthorBot || m.AuthorModerator {
		return true
	}
	for _, r := range m.AuthorRoleIDs {
		if slices.Contains(s.ExemptRoleIDs, r) {
			return true
		}
	}
	return false
}

// analysis inputs that come from the tracker.
type history struct {
	recent []RecentEntry // the author's recent messages incl. this one
	quiet  bool          // author was silent for QuietDays before this message
}

// analyze scores a message. It is pure: all state comes in through h.
func analyze(m Message, s Settings, links []Link, fps []string, h history, blocklist *DomainSet) Verdict {
	v := Verdict{Copies: []MessageRef{{ChannelID: m.ChannelID, MessageID: m.ID}}}
	add := func(sig Signal) { v.Signals = append(v.Signals, sig) }

	if s.ScamLinksEnabled {
		for _, l := range links {
			if matchList(s.AllowDomains, l.Host) != "" {
				continue
			}
			switch {
			case matchList(s.DenyDomains, l.Host) != "":
				add(Signal{Kind: SignalDeniedDomain, Detail: l.Domain, Weight: 3})
			case blocklist != nil && blocklist.Match(l.Host) != "":
				add(Signal{Kind: SignalBlockedDomain, Detail: l.Domain, Weight: 3})
			case Lookalike(l.Host) != "":
				add(Signal{Kind: SignalLookalike, Detail: l.Domain, Weight: 3})
			default:
				if shown, ok := maskedMismatch(l); ok {
					add(Signal{Kind: SignalMaskedLink, Detail: shown + " → " + l.Domain, Weight: 2})
				}
			}
		}
		if t := matchTemplates(m.Content, len(links) > 0); len(t) > 0 {
			// Scam wording with a link is strong; several patterns at once
			// (a mass ping plus "free nitro") are the full hacked-account spam.
			w := 1
			if len(links) > 0 {
				w = 2
			}
			w = min(w+len(t)-1, 3)
			add(Signal{Kind: SignalScamTemplate, Detail: strings.Join(t, ","), Weight: w})
		}
	}

	if s.BurstEnabled && len(fps) > 0 {
		var best string
		var bestChannels []snowflake.ID
		for _, fp := range fps {
			var chans []snowflake.ID
			for _, e := range h.recent {
				if slices.Contains(e.Fingerprints, fp) && !slices.Contains(chans, e.ChannelID) {
					chans = append(chans, e.ChannelID)
				}
			}
			if len(chans) > len(bestChannels) {
				best, bestChannels = fp, chans
			}
		}
		if len(bestChannels) >= s.BurstChannels {
			// Cross-posted text alone is only worth an alert; with a link,
			// a file or a mass ping it is the classic hacked-account spam.
			w := 1
			if len(links) > 0 || len(m.Attachments) > 0 || strings.Contains(m.Content, "@everyone") || strings.Contains(m.Content, "@here") {
				w = 3
			}
			add(Signal{Kind: SignalBurst, Detail: strings.SplitN(best, ":", 2)[0], Weight: w})
			v.Fingerprint, v.Channels = best, bestChannels
			for _, e := range h.recent {
				if slices.Contains(e.Fingerprints, best) && e.MessageID != m.ID {
					v.Copies = append(v.Copies, MessageRef{ChannelID: e.ChannelID, MessageID: e.MessageID})
				}
			}
		}
	}

	strong := 0
	for _, sig := range v.Signals {
		strong += sig.Weight
	}
	// A quiet member posting a link adds weight to other signals but is
	// never enough on its own: returning members share links all the time.
	if h.quiet && len(links) > 0 && strong > 0 {
		add(Signal{Kind: SignalQuietAccount, Weight: 1})
	}
	for _, sig := range v.Signals {
		v.Score += sig.Weight
	}

	switch {
	case v.Score >= scoreQuarantine:
		v.Response = ResponseQuarantine
	case v.Score >= scoreDelete:
		v.Response = ResponseDelete
	case v.Score >= scoreAlert:
		v.Response = ResponseAlert
	}
	v.Kind = KindScam
	for _, sig := range v.Signals {
		if sig.Kind == SignalBurst {
			v.Kind = KindCompromised
		}
	}
	v.Domains = uniqueDomains(links)
	if len(v.Channels) == 0 {
		v.Channels = []snowflake.ID{m.ChannelID}
	}
	return v
}

// AccountAge returns how old an account is, from its snowflake ID.
func AccountAge(userID snowflake.ID, now time.Time) time.Duration {
	return now.Sub(userID.Time())
}

// CanVerify reports whether a member may pass the verification gate, and if
// not, when they can.
func CanVerify(s Settings, userID snowflake.ID, now time.Time) (bool, time.Time) {
	min := time.Duration(s.VerifyMinAccountAgeHours) * time.Hour
	if AccountAge(userID, now) >= min {
		return true, time.Time{}
	}
	return false, userID.Time().Add(min)
}
