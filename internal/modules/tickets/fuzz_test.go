package tickets

import (
	"strings"
	"testing"
	"time"
)

// FuzzTranscript checks that nothing a member writes can reach the transcript
// page as markup: every user value is escaped by html/template.
func FuzzTranscript(f *testing.F) {
	for _, s := range []string{
		"<script>alert(1)</script>", "<img src=x onerror=alert(1)>", "</style><svg onload=1>",
		"\"><a href=javascript:1>", "plain text", "<!--", "]]><![CDATA[",
	} {
		f.Add(s, s)
	}
	f.Fuzz(func(t *testing.T, content, name string) {
		// Markers around each value let the check find exactly what the
		// template made of it.
		const mark = "QZXQ"
		if strings.Contains(content+name, mark) {
			return
		}
		content, name = mark+content+mark, mark+name+mark
		at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
		gz, err := RenderTranscript(
			TranscriptInfo{Server: name, ClosedAt: at, Ticket: Ticket{CloseReason: content}},
			[]TranscriptMessage{{ID: 1, AuthorID: 2, AuthorName: name, Content: content, Attachments: []string{name}, At: at}},
			at.Add(time.Hour),
		)
		if err != nil {
			t.Fatal(err)
		}
		page, err := Transcript{HTMLGzip: gz}.HTML()
		if err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(string(page), mark)
		for i := 1; i < len(parts); i += 2 { // the text between a pair of markers
			if strings.ContainsAny(parts[i], "<>\"") {
				t.Fatalf("user value rendered unescaped: %q", parts[i])
			}
		}
		if strings.Contains(strings.ToLower(string(page)), "<script") {
			t.Fatal("the transcript contains a script tag")
		}
	})
}
