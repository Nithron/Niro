package tickets

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// TranscriptMessage is one message in a ticket, as the platform reports it.
type TranscriptMessage struct {
	ID          snowflake.ID
	AuthorID    snowflake.ID
	AuthorName  string
	AuthorBot   bool
	Content     string
	Attachments []string // file names
	At          time.Time
}

// TranscriptInfo is the header of a transcript.
type TranscriptInfo struct {
	Server   string
	Ticket   Ticket
	ClosedAt time.Time
	// Truncated: the ticket had more messages than a transcript keeps.
	Truncated bool
}

// MaxTranscriptMessages caps a transcript (a ticket rarely has more).
const MaxTranscriptMessages = 5000

// transcriptTokens are the Nithron tokens the page uses, light and dark, with
// the values of the vendored line.css (transcript_test.go keeps them equal).
// The page loads nothing, so the faces are the system's own.
var transcriptTokens = struct{ light, dark [][2]string }{
	light: [][2]string{{"page", "#f6f7f8"}, {"ink", "#131517"}, {"ink-2", "#4c5056"}, {"dim", "#62666c"}, {"hairline", "#dfe1e4"}, {"rule", "#c2c5ca"}, {"status-earlier-text", "#916304"}},
	dark:  [][2]string{{"page", "#0c0d0f"}, {"ink", "#eceef0"}, {"ink-2", "#a9adb3"}, {"dim", "#84888e"}, {"hairline", "#1f2124"}, {"rule", "#34373c"}, {"status-earlier-text", "#f0c551"}},
}

func cssVars(vals [][2]string) string {
	var b strings.Builder
	for _, v := range vals {
		b.WriteString("--")
		b.WriteString(v[0])
		b.WriteByte(':')
		b.WriteString(v[1])
		b.WriteByte(';')
	}
	return b.String()
}

// transcriptStyle is the page in the Nithron line identity: one ground, ruled
// rows, ink text, times and IDs in the mono face, both themes by the system.
var transcriptStyle = `:root{color-scheme:light dark;` + cssVars(transcriptTokens.light) +
	`--font-sans:ui-sans-serif,system-ui,-apple-system,"Segoe UI",sans-serif;--font-mono:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
@media (prefers-color-scheme:dark){:root{` + cssVars(transcriptTokens.dark) + `}}
*{box-sizing:border-box}
body{margin:0;background:var(--page);color:var(--ink);font:400 1.0625rem/1.5 var(--font-sans)}
.w{max-width:60rem;margin:0 auto;padding:48px 24px}
header{padding-bottom:24px;border-bottom:1px solid var(--rule)}
.k{margin:0 0 8px;color:var(--dim);font:400 .8125rem/1.5 var(--font-mono)}
h1{margin:0 0 16px;font-size:1.25rem;line-height:1.25;font-weight:560;letter-spacing:-.012em}
dl{display:grid;grid-template-columns:max-content 1fr;gap:4px 24px;margin:0;font-size:.9375rem}
dt{color:var(--dim)}dd{margin:0;overflow-wrap:anywhere}
.mono,time{font-family:var(--font-mono);font-size:.875rem}
ol{list-style:none;margin:0;padding:0}
li{padding:12px 0;border-bottom:1px solid var(--hairline)}
.h{display:flex;flex-wrap:wrap;gap:4px 12px;align-items:baseline}
.a{font-weight:560}.bot,.l{color:var(--dim);font:400 .8125rem/1.5 var(--font-mono)}
time,.id{color:var(--dim)}
.c{margin-top:2px;white-space:pre-wrap;overflow-wrap:anywhere}
.f{color:var(--ink-2);font-size:.9375rem}
.note{display:flex;gap:12px;align-items:center;padding:12px 0;border-bottom:1px solid var(--hairline);color:var(--status-earlier-text)}
.sq{flex:none;width:10px;height:10px;border:2px solid currentColor}
.e{padding:12px 0;color:var(--dim)}
footer{padding-top:24px;color:var(--dim);font-size:.875rem}`

// The template escapes every value (html/template); the page has no
// scripts, no external resources and is served with a sandbox CSP.
var transcriptTmpl = template.Must(template.New("t").Funcs(template.FuncMap{
	"ts":    func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
	"iso":   func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
	"style": func() template.CSS { return template.CSS(transcriptStyle) },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Ticket #{{.Info.Ticket.Number}} · {{.Info.Server}}</title>
<style>{{style}}</style></head><body><div class="w">
<header><p class="k">Ticket transcript</p>
<h1>Ticket #{{.Info.Ticket.Number}}{{if .Info.Ticket.CategoryLabel}} · {{.Info.Ticket.CategoryLabel}}{{end}}</h1>
<dl><dt>Server</dt><dd>{{.Info.Server}}</dd>
<dt>Opened by</dt><dd>{{.Info.Ticket.OpenerName}} <span class="mono id">{{.Info.Ticket.OpenerID}}</span> · <time datetime="{{iso .Info.Ticket.CreatedAt}}">{{ts .Info.Ticket.CreatedAt}}</time></dd>
{{if .Info.Ticket.ClaimedByName}}<dt>Claimed by</dt><dd>{{.Info.Ticket.ClaimedByName}}</dd>{{end}}
<dt>Closed by</dt><dd>{{.Info.Ticket.ClosedByName}} · <time datetime="{{iso .Info.ClosedAt}}">{{ts .Info.ClosedAt}}</time></dd>
{{if .Info.Ticket.CloseReason}}<dt>Reason</dt><dd>{{.Info.Ticket.CloseReason}}</dd>{{end}}
<dt>Messages</dt><dd class="mono">{{len .Messages}}</dd></dl></header>
<main>
{{if .Info.Truncated}}<p class="note"><span class="sq" aria-hidden="true"></span>This ticket had more than {{.Max}} messages; only the first {{.Max}} are kept.</p>{{end}}
{{if .Messages}}<ol>
{{range .Messages}}<li><div class="h"><span class="a">{{.AuthorName}}</span>{{if .AuthorBot}}<span class="bot">bot</span>{{end}}<time datetime="{{iso .At}}">{{ts .At}}</time></div>
{{if .Content}}<div class="c">{{.Content}}</div>{{end}}
{{range .Attachments}}<div class="f"><span class="l">File</span> {{.}}</div>{{end}}</li>
{{end}}</ol>{{else}}<p class="e">No messages.</p>{{end}}
</main>
<footer>Transcript by Niro. Kept until <time datetime="{{iso .Expires}}">{{ts .Expires}}</time>, then deleted.</footer>
</div></body></html>
`))

// RenderTranscript writes the transcript page and returns it gzip-compressed.
func RenderTranscript(info TranscriptInfo, msgs []TranscriptMessage, expires time.Time) ([]byte, error) {
	var page bytes.Buffer
	err := transcriptTmpl.Execute(&page, struct {
		Info     TranscriptInfo
		Messages []TranscriptMessage
		Expires  time.Time
		Max      int
	}{info, msgs, expires, MaxTranscriptMessages})
	if err != nil {
		return nil, fmt.Errorf("rendering transcript: %w", err)
	}
	var gz bytes.Buffer
	w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	if _, err := w.Write(page.Bytes()); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return gz.Bytes(), nil
}

// Gunzip returns the transcript's HTML.
func (t Transcript) HTML() ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(t.HTMLGzip))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	// Transcripts are produced by Niro; still, never inflate more than 64 MiB.
	return io.ReadAll(io.LimitReader(r, 64<<20))
}
