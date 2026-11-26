package guardian

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// DefaultFeedURL is the scam-domain list Niro uses unless configured
// otherwise: nikolaischunk/discord-phishing-links (MIT licence). See
// docs/research/2026-11-scam-domain-feeds.md.
const DefaultFeedURL = "https://raw.githubusercontent.com/nikolaischunk/discord-phishing-links/main/domain-list.json"

// Feed limits.
const (
	maxFeedBytes   = 16 << 20
	maxFeedDomains = 500_000
	feedInterval   = 6 * time.Hour
)

// FeedUpdater keeps the scam-domain blocklist current. It downloads only
// the configured lists, never anything users post.
type FeedUpdater struct {
	Store  Store
	Set    *DomainSet
	URLs   []string
	Client *http.Client
	Log    *slog.Logger
	Now    func() time.Time
	// Done, if set, is told after each round whether every feed could be
	// checked and refreshed (job metrics).
	Done func(error)
}

// Run loads the stored list, refreshes stale feeds, and repeats every few
// hours until ctx is done.
func (f *FeedUpdater) Run(ctx context.Context) {
	if f.Now == nil {
		f.Now = time.Now
	}
	if f.Client == nil {
		f.Client = &http.Client{Timeout: 60 * time.Second}
	}
	f.reload(ctx)
	for {
		changed := false
		var failed error
		for _, u := range f.URLs {
			last, err := f.Store.FeedUpdatedAt(ctx, u)
			if err != nil {
				f.Log.WarnContext(ctx, "reading feed status failed", "feed", u, "err", err)
				failed = err
				continue
			}
			if f.Now().Sub(last) < feedInterval {
				continue
			}
			n, err := f.update(ctx, u)
			if err != nil {
				f.Log.WarnContext(ctx, "updating scam-domain feed failed", "feed", u, "err", err)
				failed = err
				continue
			}
			f.Log.InfoContext(ctx, "scam-domain feed updated", "feed", u, "domains", n)
			changed = true
		}
		if changed {
			f.reload(ctx)
		}
		if f.Done != nil && ctx.Err() == nil {
			f.Done(failed)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Minute):
			// Other processes may have updated the list.
			f.reload(ctx)
		}
	}
}

func (f *FeedUpdater) reload(ctx context.Context) {
	domains, err := f.Store.ScamDomains(ctx)
	if err != nil {
		f.Log.WarnContext(ctx, "loading scam domains failed", "err", err)
		return
	}
	f.Set.Replace(domains)
}

func (f *FeedUpdater) update(ctx context.Context, url string) (int, error) {
	if !strings.HasPrefix(url, "https://") {
		return 0, fmt.Errorf("feed URL must use https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Niro-Discord-Bot (scam-domain feed)")
	resp, err := f.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("feed returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	if err != nil {
		return 0, err
	}
	if len(body) > maxFeedBytes {
		return 0, fmt.Errorf("feed is larger than %d bytes", maxFeedBytes)
	}
	domains, err := ParseFeed(body)
	if err != nil {
		return 0, err
	}
	if len(domains) == 0 {
		// An empty list is far more likely a broken feed than a safe internet.
		return 0, fmt.Errorf("feed contained no domains")
	}
	return f.Store.ReplaceFeedDomains(ctx, url, domains)
}

// ParseFeed reads a domain list: JSON {"domains": [...]}, a JSON array, or
// plain text with one domain per line (# comments allowed).
func ParseFeed(body []byte) ([]string, error) {
	body = bytes.TrimSpace(body)
	var raw []string
	switch {
	case bytes.HasPrefix(body, []byte("{")):
		var obj struct {
			Domains []string `json:"domains"`
		}
		if err := json.Unmarshal(body, &obj); err != nil {
			return nil, fmt.Errorf("parsing feed: %w", err)
		}
		raw = obj.Domains
	case bytes.HasPrefix(body, []byte("[")):
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, fmt.Errorf("parsing feed: %w", err)
		}
	default:
		sc := bufio.NewScanner(bytes.NewReader(body))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			raw = append(raw, strings.Fields(line)[0])
		}
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		h, ok := NormalizeHost(d)
		if !ok || isOfficial(RegistrableDomain(h)) {
			continue // never let a feed block Discord itself
		}
		if _, dup := seen[h]; dup {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
		if len(out) >= maxFeedDomains {
			break
		}
	}
	return out, nil
}
