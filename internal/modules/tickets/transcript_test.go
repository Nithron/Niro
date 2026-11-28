package tickets

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// tokenBlock returns the custom properties of the first block after marker.
func tokenBlock(t *testing.T, css, marker string) map[string]string {
	t.Helper()
	i := strings.Index(css, marker)
	if i < 0 {
		t.Fatalf("line.css has no %q", marker)
	}
	block := css[i : i+strings.Index(css[i:], "}")]
	vals := map[string]string{}
	for _, m := range regexp.MustCompile(`--([a-z0-9-]+):\s*([^;]+);`).FindAllStringSubmatch(block, -1) {
		vals[m[1]] = strings.TrimSpace(m[2])
	}
	return vals
}

// The page's colours are the Nithron tokens Niro vendors, in both themes.
func TestTranscriptUsesTheTokens(t *testing.T) {
	raw, err := os.ReadFile("../../../web/src/vendor/nithron-tokens/line.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(raw)
	for name, set := range map[string]struct {
		vals   [][2]string
		marker string
	}{
		"light": {transcriptTokens.light, ":root {"},
		"dark":  {transcriptTokens.dark, "@media (prefers-color-scheme: dark)"},
	} {
		want := tokenBlock(t, css, set.marker)
		for _, v := range set.vals {
			if want[v[0]] != v[1] {
				t.Errorf("%s --%s: transcript %s, line.css %q", name, v[0], v[1], want[v[0]])
			}
		}
	}
}

func TestTranscriptPage(t *testing.T) {
	at := time.Date(2026, 11, 28, 9, 30, 0, 0, time.UTC)
	gz, err := RenderTranscript(TranscriptInfo{
		Server: "Test <b>server</b>", Ticket: Ticket{Number: 7, OpenerName: "ana", OpenerID: 42, CreatedAt: at, ClosedByName: "mod"},
		ClosedAt: at, Truncated: true,
	}, []TranscriptMessage{{AuthorName: "ana", Content: "<script>x</script>", Attachments: []string{"log.txt"}, At: at}, {AuthorName: "Niro", AuthorBot: true, At: at}}, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := gzip.NewReader(bytes.NewReader(gz))
	page, _ := io.ReadAll(r)
	s := string(page)
	for _, want := range []string{
		"Test &lt;b&gt;server&lt;/b&gt;", "&lt;script&gt;x&lt;/script&gt;", // escaped
		`<span class="mono id">42</span>`, `<time datetime="2026-11-28T09:30:00Z">2026-11-28 09:30 UTC</time>`,
		`<span class="bot">bot</span>`, `<span class="l">File</span> log.txt`, "only the first 5000 are kept",
		"prefers-color-scheme:dark", "--page:#0c0d0f",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	for _, banned := range []string{"<script", "http://", "https://", "#5865f2", "#1e1f22", "📎"} {
		if strings.Contains(s, banned) {
			t.Errorf("page has %q", banned)
		}
	}
}
