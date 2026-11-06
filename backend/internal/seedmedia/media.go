// Package seedmedia contains the rights-cleared illustrations for the pilot places.
package seedmedia

import "embed"

// Files is read-only and separate from curator-uploaded media.
//
//go:embed images/*.jpg
var Files embed.FS
