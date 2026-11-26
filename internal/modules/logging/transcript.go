package logging

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// TranscriptLabels are the translated words in a bulk-delete file.
type TranscriptLabels struct {
	Header     []string // first lines (server, channel, count…)
	NotCached  string   // for a message that was not in the cache
	Files      string   // prefix for attachment names
	NoContent  string   // the server turned message content off
	EmptyTexts string   // a cached message with no text
}

// WriteTranscript writes a plain-text file of bulk-deleted messages, oldest
// first: "[time] name (id): text". Messages that were not cached appear by
// ID only.
func WriteTranscript(w io.Writer, l TranscriptLabels, msgs []CachedMessage, ids []snowflake.ID, hidden bool) error {
	var b strings.Builder
	for _, h := range l.Header {
		b.WriteString(h)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	byID := make(map[snowflake.ID]CachedMessage, len(msgs))
	for _, m := range msgs {
		byID[m.ID] = m
	}
	all := slices.Clone(ids)
	for _, m := range msgs {
		if !slices.Contains(all, m.ID) {
			all = append(all, m.ID)
		}
	}
	slices.SortFunc(all, compareIDs)
	for _, id := range all {
		m, ok := byID[id]
		if !ok {
			fmt.Fprintf(&b, "[%s] %s: %s\n", id.Time().UTC().Format(time.DateTime), id, l.NotCached)
			continue
		}
		text := m.Content
		switch {
		case hidden:
			text = l.NoContent
		case text == "" && len(m.Attachments) == 0:
			text = l.EmptyTexts
		}
		// Indent continuation lines so each message stays one block.
		text = strings.ReplaceAll(text, "\n", "\n    ")
		fmt.Fprintf(&b, "[%s] %s (%s): %s", m.CreatedAt.UTC().Format(time.DateTime), m.AuthorName, m.AuthorID, text)
		if len(m.Attachments) > 0 {
			fmt.Fprintf(&b, " [%s %s]", l.Files, strings.Join(m.Attachments, ", "))
		}
		b.WriteByte('\n')
	}
	_, err := io.WriteString(w, b.String())
	return err
}
