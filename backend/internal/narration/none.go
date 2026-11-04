package narration

import "context"

// None produces no audio; clients fall back to on-device speech.
type None struct{}

func (None) Synthesize(context.Context, string) (*Audio, error) { return nil, nil }
