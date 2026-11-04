package routing

// DecodePolyline6 decodes a Google-style encoded polyline at 1e-6 precision into [lng, lat] pairs.
func DecodePolyline6(s string) [][2]float64 {
	var out [][2]float64
	var lat, lng int64
	i := 0
	next := func() (int64, bool) {
		var result, shift int64
		for {
			if i >= len(s) {
				return 0, false
			}
			b := int64(s[i]) - 63
			i++
			result |= (b & 0x1f) << shift
			shift += 5
			if b < 0x20 {
				break
			}
		}
		if result&1 != 0 {
			return ^(result >> 1), true
		}
		return result >> 1, true
	}
	for i < len(s) {
		dLat, ok := next()
		if !ok {
			break
		}
		dLng, ok := next()
		if !ok {
			break
		}
		lat += dLat
		lng += dLng
		out = append(out, [2]float64{float64(lng) / 1e6, float64(lat) / 1e6})
	}
	return out
}
