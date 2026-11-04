// Package narration turns moment text into spoken audio.
package narration

import "context"

// Audio is synthesized speech.
type Audio struct {
	Data            []byte
	MimeType        string
	DurationSeconds *float64
}

// Narrator synthesizes speech. A nil Audio with nil error means the provider produces nothing.
type Narrator interface {
	Synthesize(ctx context.Context, text string) (*Audio, error)
}
