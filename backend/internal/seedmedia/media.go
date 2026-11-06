// Package seedmedia contains the reviewed photographs and documents for the pilot places.
package seedmedia

import "embed"

// Files is read-only and separate from curator-uploaded media.
//
//go:embed images/*.jpg documents/*.pdf
var Files embed.FS
