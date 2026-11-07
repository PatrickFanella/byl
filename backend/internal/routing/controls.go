package routing

import (
	"fmt"
	"math"
	"slices"
	"sort"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
)

const maxControls = 256
const referenceToleranceM = 50.0

// StoryLeg binds provider legs to the itinerary, including a non-narrated return.
type StoryLeg struct {
	FromID, ToID string
	From, To     int
	Return       bool
}

// Plan retains story boundaries separately from non-story control points.
type Plan struct {
	Controls        []Waypoint
	Legs            []StoryLeg
	Controlled      bool
	FollowReference bool
}

// BuildPlan leaves historical-site routes compatible. Once an approved arrival
// or corridor is present, every stop requires an allowed arrival for this mode.
func BuildPlan(tour *models.TourDetail, mode models.TravelMode) (Plan, error) {
	plan := Plan{}
	if !mode.Valid() || !slices.Contains(tour.Modes, mode) {
		return plan, fmt.Errorf("navigation_mode_unavailable")
	}
	if len(tour.Stops) < 2 {
		return plan, ErrTooFewPoints
	}
	var intent *models.CorridorIntent
	if tour.Corridor != nil {
		intent = tour.Corridor.Intent
	}
	plan.Controlled = intent != nil
	plan.FollowReference = intent != nil
	for _, stop := range tour.Stops {
		plan.Controlled = plan.Controlled || stop.Place.ArrivalPoint != nil
	}
	for i, stop := range tour.Stops {
		point := Point{Lat: stop.Place.Latitude, Lng: stop.Place.Longitude}
		if plan.Controlled {
			selection := stop.Place.SelectArrival(mode)
			if selection.NavigationPoint == nil {
				return Plan{}, fmt.Errorf("arrival_%s: %s stop %d", selection.Eligibility, mode, stop.Position)
			}
			point = Point{Lat: selection.NavigationPoint.Lat, Lng: selection.NavigationPoint.Lng}
		}
		if !finitePoint(point) || stop.Position != i+1 || stop.ID == "" {
			return Plan{}, fmt.Errorf("invalid_story_stop: %d", i+1)
		}
		plan.Controls = append(plan.Controls, Waypoint{Point: point, Break: true})
		if i > 0 {
			plan.Legs = append(plan.Legs, StoryLeg{FromID: tour.Stops[i-1].ID, ToID: stop.ID, From: i, To: i + 1})
		}
	}
	if intent == nil {
		return plan, nil
	}
	if len(intent.Validate()) != 0 || intent.ValidateStops(tour) != nil {
		return Plan{}, fmt.Errorf("invalid_corridor_intent")
	}
	var selection *models.ModeCorridor
	for i := range intent.Routes {
		if intent.Routes[i].Mode == mode {
			selection = &intent.Routes[i]
		}
	}
	if selection == nil {
		return Plan{}, fmt.Errorf("corridor_mode_unavailable: %s", mode)
	}
	return corridorPlan(plan, tour, intent, *selection)
}

type pathPoint struct {
	point    Point
	progress float64
}

type occurrence struct {
	points []pathPoint
}

func corridorPlan(plan Plan, tour *models.TourDetail, intent *models.CorridorIntent, selection models.ModeCorridor) (Plan, error) {
	segments := map[string]models.CorridorSegment{}
	for _, segment := range intent.Segments {
		segments[segment.ID] = segment
	}
	var path []pathPoint
	var occurrences []occurrence
	var startNode, endNode string
	for i, traversal := range selection.Path {
		segment := segments[traversal.SegmentID]
		for _, access := range segment.Access {
			if access.Mode == selection.Mode && (access.Status == "closed" || access.Status == "restricted") {
				return Plan{}, fmt.Errorf("corridor_access_%s: %s segment %s", access.Status, selection.Mode, segment.ID)
			}
		}
		coordinates := slices.Clone(segment.Geometry.Coordinates)
		from, to := segment.FromNode, segment.ToNode
		if traversal.Reverse {
			slices.Reverse(coordinates)
			from, to = to, from
		}
		if i == 0 {
			startNode = from
		}
		endNode = to
		var current occurrence
		for _, coordinate := range coordinates {
			point := Point{Lat: coordinate[1], Lng: coordinate[0]}
			progress := 0.0
			if len(path) > 0 {
				last := path[len(path)-1]
				progress = last.progress + distance(last.point, point)
			}
			p := pathPoint{point: point, progress: progress}
			path = append(path, p)
			current.points = append(current.points, p)
		}
		occurrences = append(occurrences, current)
	}
	closed := startNode == endNode
	total := path[len(path)-1].progress
	if total < 1 || distance(plan.Controls[0].Point, path[0].point) > referenceToleranceM {
		return Plan{}, fmt.Errorf("corridor_start_not_at_first_arrival")
	}
	end := plan.Controls[len(plan.Controls)-1].Point
	if closed {
		end = plan.Controls[0].Point
	}
	if distance(end, path[len(path)-1].point) > referenceToleranceM {
		return Plan{}, fmt.Errorf("corridor_end_not_at_arrival")
	}
	positions := map[string]int{}
	for i, stop := range tour.Stops {
		positions[stop.ID] = i
	}
	type control struct {
		pathPoint
		story int // -1 for reference/shape, otherwise itinerary index
		shape bool
	}
	var controls []control
	for _, p := range path {
		controls = append(controls, control{pathPoint: p, story: -1})
	}
	lower, upper := make([]float64, len(tour.Stops)), make([]float64, len(tour.Stops))
	for i := range upper {
		upper[i] = total
	}
	lastShape := -1.0
	for _, shape := range selection.ShapingPoints {
		point := Point{Lat: *shape.Latitude, Lng: *shape.Longitude}
		candidates := projectOccurrences(point, []occurrence{occurrences[shape.TraversalIndex]})
		if len(candidates) != 1 || candidates[0] < lastShape {
			return Plan{}, fmt.Errorf("ambiguous_or_reversed_shaping_point")
		}
		progress := candidates[0]
		lastShape = progress
		after, before := positions[shape.AfterStopID], positions[shape.BeforeStopID]
		upper[after] = min(upper[after], progress)
		lower[before] = max(lower[before], progress)
		controls = append(controls, control{pathPoint: pathPoint{point, progress}, story: -1, shape: true})
	}
	last := -1.0
	stopProgress := make([]float64, len(tour.Stops))
	for i, stop := range plan.Controls {
		var candidates []float64
		switch {
		case i == 0:
			candidates = []float64{0}
		case i == len(plan.Controls)-1 && !closed:
			candidates = []float64{total}
		default:
			candidates = projectOccurrences(stop.Point, occurrences)
		}
		var eligible []float64
		for _, p := range candidates {
			if p > last+0.01 && p >= lower[i]-0.01 && p <= upper[i]+0.01 && (!closed || p < total-0.01) {
				eligible = append(eligible, p)
			}
		}
		if len(eligible) != 1 {
			return Plan{}, fmt.Errorf("ambiguous_or_missing_stop_occurrence: %d", i+1)
		}
		last = eligible[0]
		stopProgress[i] = last
		controls = append(controls, control{pathPoint: pathPoint{stop.Point, last}, story: i})
	}
	for _, shape := range selection.ShapingPoints {
		point := Point{Lat: *shape.Latitude, Lng: *shape.Longitude}
		progress := projectOccurrences(point, []occurrence{occurrences[shape.TraversalIndex]})[0]
		if progress <= stopProgress[positions[shape.AfterStopID]]+0.01 || progress >= stopProgress[positions[shape.BeforeStopID]]-0.01 {
			return Plan{}, fmt.Errorf("shaping_point_not_between_story_stops")
		}
	}
	if closed {
		controls = append(controls, control{pathPoint: pathPoint{plan.Controls[0].Point, total}, story: 0})
		plan.Legs = append(plan.Legs, StoryLeg{FromID: tour.Stops[len(tour.Stops)-1].ID, ToID: tour.Stops[0].ID, From: len(tour.Stops), To: 1, Return: true})
	}
	// Keep reviewed points in place; reference vertices at precisely the same
	// progress need not create duplicate controls. Never collapse story breaks.
	sort.SliceStable(controls, func(i, j int) bool {
		a, b := math.Round(controls[i].progress*1000), math.Round(controls[j].progress*1000)
		if a != b {
			return a < b
		}
		return controls[i].story >= 0 && controls[j].story < 0 || controls[i].shape && controls[j].story < 0 && !controls[j].shape
	})
	plan.Controls = nil
	previousProgress := -1.0
	for _, p := range controls {
		if p.story < 0 && math.Abs(p.progress-previousProgress) <= 0.001 && !p.shape {
			continue
		}
		plan.Controls = append(plan.Controls, Waypoint{Point: p.point, Break: p.story >= 0})
		previousProgress = p.progress
	}
	if len(plan.Controls) > maxControls {
		return Plan{}, fmt.Errorf("corridor_control_limit: maximum %d; explicitly review a bounded reference", maxControls)
	}
	return plan, nil
}

// Project each declared occurrence independently. Repeated traversals produce
// distinct progress values; ambiguous crossings are rejected rather than chosen.
func projectOccurrences(point Point, occurrences []occurrence) []float64 {
	var result []float64
	for _, occurrence := range occurrences {
		best := math.Inf(1)
		var candidates []struct{ progress, distance float64 }
		for i := 1; i < len(occurrence.points); i++ {
			a, b := occurrence.points[i-1], occurrence.points[i]
			x := math.Remainder(b.point.Lng-a.point.Lng, 360) * math.Cos((a.point.Lat+b.point.Lat)*math.Pi/360)
			y := b.point.Lat - a.point.Lat
			px := math.Remainder(point.Lng-a.point.Lng, 360) * math.Cos((a.point.Lat+b.point.Lat)*math.Pi/360)
			py := point.Lat - a.point.Lat
			t := 0.0
			if x*x+y*y > 0 {
				t = max(0, min(1, (px*x+py*y)/(x*x+y*y)))
			}
			p := Point{Lat: a.point.Lat + t*y, Lng: a.point.Lng + t*math.Remainder(b.point.Lng-a.point.Lng, 360)}
			d := distance(point, p)
			best = min(best, d)
			candidates = append(candidates, struct{ progress, distance float64 }{a.progress + t*(b.progress-a.progress), d})
		}
		for _, candidate := range candidates {
			if candidate.distance <= referenceToleranceM && candidate.distance <= best+0.5 && (len(result) == 0 || math.Abs(result[len(result)-1]-candidate.progress) > 1) {
				result = append(result, candidate.progress)
			}
		}
	}
	return result
}

func distance(a, b Point) float64 { return geo.DistanceMeters(a.Lat, a.Lng, b.Lat, b.Lng) }

func finitePoint(p Point) bool {
	return !math.IsNaN(p.Lat) && !math.IsNaN(p.Lng) && !math.IsInf(p.Lat, 0) && !math.IsInf(p.Lng, 0) && math.Abs(p.Lat) <= 90 && math.Abs(p.Lng) <= 180
}
