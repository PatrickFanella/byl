package sources

import (
	"regexp"
	"strings"
)

var ccRe = regexp.MustCompile(`(?i)^CC[ -]?(BY(?:[ -]?(?:NC|SA|ND))*)[ -]?(\d\.\d)$`)

// MapLicense turns a Wikimedia-style licence short name into the identifier the
// content model uses. Unknown or non-free names map to "" so publishing stays blocked.
func MapLicense(short string) string {
	s := strings.TrimSpace(short)
	switch {
	case s == "":
		return ""
	case strings.EqualFold(s, "CC0") || strings.EqualFold(s, "CC0 1.0"):
		return "CC0-1.0"
	case strings.EqualFold(s, "Public domain") || strings.HasPrefix(strings.ToUpper(s), "PD") || strings.EqualFold(s, "No restrictions"):
		return "PD"
	case strings.HasPrefix(strings.ToUpper(s), "GFDL"):
		if strings.Contains(s, "1.3") {
			return "GFDL-1.3"
		}
		return "GFDL-1.2"
	}
	if m := ccRe.FindStringSubmatch(s); m != nil {
		elements := strings.ToUpper(strings.ReplaceAll(m[1], " ", "-"))
		if strings.Contains(elements, "NC") || strings.Contains(elements, "ND") {
			return ""
		}
		return "CC-" + elements + "-" + m[2]
	}
	return ""
}
