// Package geo holds the small amount of geography and naming math the API needs.
package geo

import (
	"crypto/rand"
	"math"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

const earthRadiusM = 6371000.0

// Point is a WGS84 coordinate.
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// BBox is a latitude/longitude bounding box.
type BBox struct {
	MinLat float64 `json:"min_lat"`
	MinLng float64 `json:"min_lng"`
	MaxLat float64 `json:"max_lat"`
	MaxLng float64 `json:"max_lng"`
}

// Contains reports whether the point lies inside the box.
func (b BBox) Contains(lat, lng float64) bool {
	return lat >= b.MinLat && lat <= b.MaxLat && lng >= b.MinLng && lng <= b.MaxLng
}

// DistanceMeters is the haversine great-circle distance.
func DistanceMeters(lat1, lng1, lat2, lng2 float64) float64 {
	φ1, φ2 := lat1*math.Pi/180, lat2*math.Pi/180
	dφ := (lat2 - lat1) * math.Pi / 180
	dλ := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dφ/2)*math.Sin(dφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	return 2 * earthRadiusM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// BoundingBox returns a box that fully contains the circle of radiusM around the point.
func BoundingBox(lat, lng, radiusM float64) BBox {
	dLat := radiusM / earthRadiusM * 180 / math.Pi
	cosLat := math.Cos(lat * math.Pi / 180)
	if cosLat < 1e-6 {
		cosLat = 1e-6
	}
	dLng := radiusM / (earthRadiusM * cosLat) * 180 / math.Pi
	return BBox{MinLat: lat - dLat, MinLng: lng - dLng, MaxLat: lat + dLat, MaxLng: lng + dLng}
}

// Slugify lowercases, strips accents, and joins words with single hyphens.
func Slugify(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	plain, _, err := transform.String(t, s)
	if err != nil {
		plain = s
	}
	var b strings.Builder
	lastHyphen := true
	for _, r := range strings.ToLower(plain) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

const slugAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// RandomSlug returns n random lowercase alphanumeric characters.
func RandomSlug(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = slugAlphabet[int(buf[i])%len(slugAlphabet)]
	}
	return string(buf)
}

// NearPolyline bounds a point near existing reference geometry using a local
// projection. It does not infer access or connections at visual crossings.
func NearPolyline(point [2]float64, path [][2]float64, radius float64) bool {
	for i, a := range path {
		if DistanceMeters(point[1], point[0], a[1], a[0]) <= radius {
			return true
		}
		if i == 0 {
			continue
		}
		b := path[i-1]
		x := math.Remainder(a[0]-b[0], 360) * math.Cos((a[1]+b[1])*math.Pi/360)
		y := a[1] - b[1]
		px := math.Remainder(point[0]-b[0], 360) * math.Cos((a[1]+b[1])*math.Pi/360)
		py := point[1] - b[1]
		if x*x+y*y == 0 {
			continue
		}
		t := max(0, min(1, (px*x+py*y)/(x*x+y*y)))
		lng := math.Remainder(b[0]+t*math.Remainder(a[0]-b[0], 360), 360)
		lat := b[1] + t*y
		if DistanceMeters(point[1], point[0], lat, lng) <= radius {
			return true
		}
	}
	return false
}
