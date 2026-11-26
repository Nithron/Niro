package api

import (
	"errors"
	"net/http"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core/message"
	dc "github.com/nithron/niro/internal/platform/discord"
)

func toAPIDocument(d message.Document) apigen.MessageDocument {
	out := apigen.MessageDocument{Version: d.Version, AccentColor: d.AccentColor, Blocks: make([]apigen.MessageBlock, len(d.Blocks))}
	if out.Version == 0 {
		out.Version = message.Version
	}
	for i, b := range d.Blocks {
		ab := apigen.MessageBlock{Type: apigen.MessageBlockType(b.Type)}
		if b.Text != "" {
			ab.Text = ptr(b.Text)
		}
		if b.Thumbnail != "" {
			ab.Thumbnail = ptr(b.Thumbnail)
		}
		if b.Divider {
			ab.Divider = ptr(true)
		}
		if b.Large {
			ab.Large = ptr(true)
		}
		if len(b.Images) > 0 {
			imgs := make([]struct {
				Description *string `json:"description,omitempty"`
				Url         string  `json:"url"`
			}, len(b.Images))
			for j, img := range b.Images {
				imgs[j].Url = img.URL
				if img.Description != "" {
					imgs[j].Description = ptr(img.Description)
				}
			}
			ab.Images = &imgs
		}
		if len(b.Links) > 0 {
			links := make([]struct {
				Emoji *string `json:"emoji,omitempty"`
				Label string  `json:"label"`
				Url   string  `json:"url"`
			}, len(b.Links))
			for j, l := range b.Links {
				links[j].Label, links[j].Url = l.Label, l.URL
				if l.Emoji != "" {
					links[j].Emoji = ptr(l.Emoji)
				}
			}
			ab.Links = &links
		}
		out.Blocks[i] = ab
	}
	return out
}

// fromAPIDocument converts and validates a document, leaving room for what
// the feature adds.
func fromAPIDocument(a apigen.MessageDocument, reserve message.Budget) (message.Document, error) {
	d := message.Document{Version: a.Version, AccentColor: a.AccentColor, Blocks: make([]message.Block, len(a.Blocks))}
	for i, ab := range a.Blocks {
		b := message.Block{Type: message.BlockType(ab.Type), Text: deref(ab.Text), Thumbnail: deref(ab.Thumbnail),
			Divider: deref(ab.Divider), Large: deref(ab.Large)}
		if ab.Images != nil {
			for _, img := range *ab.Images {
				b.Images = append(b.Images, message.Image{URL: img.Url, Description: deref(img.Description)})
			}
		}
		if ab.Links != nil {
			for _, l := range *ab.Links {
				b.Links = append(b.Links, message.Link{Label: l.Label, URL: l.Url, Emoji: deref(l.Emoji)})
			}
		}
		d.Blocks[i] = b
	}
	if err := d.Validate(reserve); err != nil {
		return d, documentError(err)
	}
	return d, nil
}

func documentError(err error) error {
	if errors.Is(err, message.ErrInvalid) {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Message: "+dc.ValidationMessage(err)+".")
	}
	return err
}

func ptr[T any](v T) *T { return &v }

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
