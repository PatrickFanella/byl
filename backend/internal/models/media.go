package models

import "strings"

// PublicationIssue checks copied-file rights separately from external references.
func (m MediaAsset) PublicationIssue() string {
	if m.External {
		if m.Kind != MediaKindVideo && m.Kind != MediaKindDocument {
			return "external references must be videos or documents"
		}
		if !webURL(m.PublicURL) || !webURL(m.SourceURL) || strings.TrimSpace(m.Caption) == "" || strings.TrimSpace(m.Credit) == "" {
			return "external reference needs a title, creator/publisher, destination and source URL"
		}
		return ""
	}
	if m.Kind == MediaKindVideo {
		return "video hosting is disabled; add an external video link"
	}
	if m.PublicURL == "" {
		return "file must be uploaded or fetched"
	}
	if !ReusableLicense(m.License) {
		return "file needs a reusable license"
	}
	if (m.Kind == MediaKindImage || m.Kind == MediaKindDocument) &&
		(strings.TrimSpace(m.Caption) == "" || strings.TrimSpace(m.Credit) == "" || !webURL(m.SourceURL) || !webURL(m.LicenseURL)) {
		return "file needs a caption/title, credit, source and license URLs"
	}
	return ""
}
