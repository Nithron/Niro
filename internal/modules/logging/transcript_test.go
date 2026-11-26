package logging

import (
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func TestWriteTranscript(t *testing.T) {
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	msgs := []CachedMessage{
		{ID: 30, AuthorID: 7, AuthorName: "bob", Content: "line one\nline two", CreatedAt: at.Add(time.Minute)},
		{ID: 10, AuthorID: 8, AuthorName: "eve", Attachments: []string{"a.png"}, CreatedAt: at},
	}
	labels := TranscriptLabels{Header: []string{"Bulk delete in #general", "3 messages"}, NotCached: "(not in the cache)",
		Files: "files:", NoContent: "(content logging is off)", EmptyTexts: "(no text)"}
	var b strings.Builder
	if err := WriteTranscript(&b, labels, msgs, []snowflake.ID{10, 20, 30}, false); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	want := "Bulk delete in #general\n3 messages\n\n" +
		"[2026-11-26 12:00:00] eve (8):  [files: a.png]\n" +
		"[" + snowflake.ID(20).Time().UTC().Format(time.DateTime) + "] 20: (not in the cache)\n" +
		"[2026-11-26 12:01:00] bob (7): line one\n    line two\n"
	if got != want {
		t.Errorf("transcript:\n%s\nwant:\n%s", got, want)
	}
	b.Reset()
	_ = WriteTranscript(&b, labels, msgs, []snowflake.ID{10, 30}, true)
	if strings.Contains(b.String(), "line one") || !strings.Contains(b.String(), "(content logging is off)") {
		t.Errorf("hidden transcript shows text:\n%s", b.String())
	}
}
