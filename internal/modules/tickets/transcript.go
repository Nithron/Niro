package tickets

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"html/template"
	"io"
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

// The template escapes every value (html/template); the page has no
// scripts, no external resources and is served with a sandbox CSP.
var transcriptTmpl = template.Must(template.New("t").Funcs(template.FuncMap{
	"ts": func(t time.Time) string { return t.UTC().Format("2006-01-02 15:04 UTC") },
}).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="referrer" content="no-referrer">
<title>Ticket #{{.Info.Ticket.Number}} · {{.Info.Server}}</title>
<style>
body{font:15px/1.5 system-ui,sans-serif;margin:0;background:#1e1f22;color:#dbdee1}
header{padding:20px 24px;background:#2b2d31;border-bottom:1px solid #3f4147}
h1{font-size:20px;margin:0 0 8px}dl{display:grid;grid-template-columns:auto 1fr;gap:2px 16px;margin:0;font-size:13px}
dt{color:#949ba4}dd{margin:0}main{padding:12px 24px}
.m{padding:6px 0;border-bottom:1px solid #2b2d31}.a{font-weight:600}.bot{font-size:11px;background:#5865f2;color:#fff;border-radius:3px;padding:0 4px;margin-left:4px}
.t{color:#949ba4;font-size:12px;margin-left:8px}.c{white-space:pre-wrap;word-wrap:break-word}.f{color:#949ba4;font-size:13px}
.note{color:#f5b83d;padding:8px 0}footer{padding:16px 24px;color:#949ba4;font-size:12px}
</style></head><body>
<header><h1>Ticket #{{.Info.Ticket.Number}} · {{.Info.Ticket.CategoryLabel}}</h1>
<dl><dt>Server</dt><dd>{{.Info.Server}}</dd>
<dt>Opened by</dt><dd>{{.Info.Ticket.OpenerName}} ({{.Info.Ticket.OpenerID}}) · {{ts .Info.Ticket.CreatedAt}}</dd>
{{if .Info.Ticket.ClaimedByName}}<dt>Claimed by</dt><dd>{{.Info.Ticket.ClaimedByName}}</dd>{{end}}
<dt>Closed by</dt><dd>{{.Info.Ticket.ClosedByName}} · {{ts .Info.ClosedAt}}</dd>
{{if .Info.Ticket.CloseReason}}<dt>Reason</dt><dd>{{.Info.Ticket.CloseReason}}</dd>{{end}}
<dt>Messages</dt><dd>{{len .Messages}}</dd></dl></header>
<main>
{{if .Info.Truncated}}<p class="note">This ticket had more than {{.Max}} messages; only the first {{.Max}} are kept.</p>{{end}}
{{range .Messages}}<div class="m"><span class="a">{{.AuthorName}}</span>{{if .AuthorBot}}<span class="bot">BOT</span>{{end}}<span class="t">{{ts .At}}</span>
{{if .Content}}<div class="c">{{.Content}}</div>{{end}}
{{range .Attachments}}<div class="f">📎 {{.}}</div>{{end}}</div>
{{end}}
</main>
<footer>Transcript by Niro. Kept until {{ts .Expires}}, then deleted.</footer>
</body></html>
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
