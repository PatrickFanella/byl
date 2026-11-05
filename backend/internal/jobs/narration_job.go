package jobs

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/models"
)

func (w *Worker) generateNarration(ctx context.Context, p models.GenerateNarrationPayload) error {
	m, err := w.store.GetMoment(ctx, p.MomentID, true)
	if err != nil {
		return fmt.Errorf("load moment: %w", err)
	}
	text := plainText(m.Title + "\n\n" + m.Body)
	audio, err := w.narrator.Synthesize(ctx, text)
	if err != nil {
		return err
	}
	if audio == nil {
		return nil
	}
	key := blob.KeyFor("narration", m.ID+".mp3")
	if err := w.blob.Put(ctx, key, audio.MimeType, bytes.NewReader(audio.Data), int64(len(audio.Data))); err != nil {
		return fmt.Errorf("store audio: %w", err)
	}
	asset, err := w.store.CreateMedia(ctx, models.MediaAsset{
		Kind: models.MediaKindAudio, StorageKey: key, PublicURL: w.blob.PublicURL(key), MimeType: audio.MimeType,
		DurationSeconds: audio.DurationSeconds, Caption: "Narration: " + m.Title, Credit: "Invisible Museum narration",
		License: "all-rights-reserved", Status: models.EntityStatusPublished,
	})
	if err != nil {
		_ = w.blob.Delete(ctx, key)
		return fmt.Errorf("record asset: %w", err)
	}
	return w.store.SetMomentNarration(ctx, m.ID, &asset.ID)
}

var (
	mdHeading = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	mdList    = regexp.MustCompile(`(?m)^[ \t]*(?:[-*+]|\d+\.)[ \t]+`)
	mdLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	mdImage   = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	mdEmph    = regexp.MustCompile(`(\*\*|__|\*|_|~~|` + "`" + `)`)
)

// plainText strips the Markdown the narrator should not read aloud.
func plainText(md string) string {
	s := mdImage.ReplaceAllString(md, "")
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdHeading.ReplaceAllString(s, "")
	s = mdList.ReplaceAllString(s, "")
	s = mdEmph.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
