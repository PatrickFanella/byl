package sources

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"unicode"

	"golang.org/x/net/html"

	"invisible-museum/backend/internal/models"
)

// DiscoveryPolicy selects trusted starting pages and permitted search adapters.
// Submitted links and followed references must share a configured seed's hostname.
type DiscoveryPolicy struct{ Providers, Seeds []string }

// DefaultDiscoverySeeds are entry points, not verified historical claims or media permissions.
var DefaultDiscoverySeeds = []string{"https://dghistory.org/", "https://en.wikipedia.org/wiki/Downers_Grove,_Illinois", "https://en.wikipedia.org/wiki/DuPage_County,_Illinois"}

// ValidDiscoveryProvider keeps automatic discovery separate from broad legacy imports.
func ValidDiscoveryProvider(name string) bool {
	return name == "wikipedia_search" || name == "loc" || name == "internet_archive"
}

// Discovery coordinates existing archives with bounded page inspection and reference following.
type Discovery struct {
	registry *Registry
	policy   DiscoveryPolicy
}

// NewDiscovery builds a coordinator; no network calls occur until a curator starts research.
func NewDiscovery(registry *Registry, policy DiscoveryPolicy) *Discovery {
	return &Discovery{registry: registry, policy: policy}
}

// DiscoveryTerms normalizes names into word boundaries, so a namesake substring is not a match.
func DiscoveryTerms(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

func phrase(text, term string) bool {
	return term != "" && strings.Contains(" "+DiscoveryTerms(text)+" ", " "+DiscoveryTerms(term)+" ")
}

// DiscoveryReasons requires explicit area and topic/lead support in the returned content.
// The originating query is never used as proof that a hit matches its requested area.
func DiscoveryReasons(in models.RouteBriefInput, item models.DiscoveryItem, scope *models.DiscoveryScope) []string {
	text := item.Title + " " + item.Text
	areaOK := phrase(text, in.Area) || phrase(item.Area, in.Area)
	if scope != nil && item.Lat != nil && item.Lng != nil {
		b := scope.BBox
		if *item.Lat < b.MinLat || *item.Lat > b.MaxLat || *item.Lng < b.MinLng || *item.Lng > b.MaxLng {
			return nil
		}
		areaOK = true
	}
	if !areaOK {
		return nil
	}
	reasons := []string{"area: " + in.Area}
	leadMatched := false
	for _, lead := range append(append([]string{}, in.PlaceLeads...), in.EventLeads...) {
		if phrase(text, lead) {
			leadMatched = true
			reasons = append(reasons, "lead: "+lead)
		}
	}
	stop := map[string]bool{"a": true, "an": true, "and": true, "of": true, "the": true, "in": true, "on": true, "for": true, "history": true, "historical": true, "tour": true}
	areaWords := map[string]bool{}
	for _, w := range strings.Fields(DiscoveryTerms(in.Area)) {
		areaWords[w] = true
	}
	topicTerms, topicMatches := 0, 0
	for _, word := range strings.Fields(DiscoveryTerms(in.Topic)) {
		if len(word) < 3 || stop[word] || areaWords[word] {
			continue
		}
		topicTerms++
		if phrase(text, word) || (strings.HasSuffix(word, "s") && len(word) > 4 && phrase(text, strings.TrimSuffix(word, "s"))) {
			topicMatches++
			reasons = append(reasons, "topic: "+word)
		}
	}
	if !leadMatched && (topicTerms == 0 || topicMatches != topicTerms) {
		return nil
	}
	return reasons
}

func discoveryQueries(in models.RouteBriefInput) []string {
	quote := func(s string) string { return `"` + cut(DiscoveryTerms(s), 120) + `"` }
	base := quote(in.Area) + " " + quote(in.Topic)
	out := []string{base}
	for _, lead := range append(append([]string{}, in.PlaceLeads...), in.EventLeads...) {
		if len(out) == 3 {
			break
		}
		out = append(out, quote(in.Area)+" "+quote(lead)+" "+quote(in.Topic))
	}
	return out
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// Run reuses known content first, inspects at most four starting pages and two
// references, and runs at most three archive queries. Every HTTP hop must reserve
// a durable allowance before sending. Cancellation discards no committed earlier stage.
func (d *Discovery) Run(ctx context.Context, in models.RouteBriefInput, scope *models.DiscoveryScope, known []models.DiscoveryItem, retained []models.SourceEvidence, reserve func(context.Context) error) (models.DiscoveryReport, error) {
	report := models.DiscoveryReport{Items: []models.DiscoveryItem{}, Sources: []models.DiscoverySource{}, Providers: []models.DiscoveryProvider{}, Gaps: []string{}}
	ctx = context.WithValue(ctx, discoveryBudgetKey{}, discoveryBudget(reserve))
	seen := map[string]int{}
	add := func(item models.DiscoveryItem, provider, query, origin string) {
		reasons := DiscoveryReasons(in, item, scope)
		if len(reasons) == 0 {
			return
		}
		key := item.EntityID
		if key == "" {
			var err error
			_, _, key, err = NormalizeEvidenceURL(item.URL)
			if err != nil {
				return
			}
		}
		o := models.DiscoveryOrigin{Provider: provider, Query: query, URL: origin, Reasons: reasons}
		if i, ok := seen[key]; ok {
			if len(report.Items[i].Origins) < 6 {
				report.Items[i].Origins = append(report.Items[i].Origins, o)
			}
			return
		}
		if len(report.Items) >= 12 {
			return
		}
		item.Title = cut(item.Title, 500)
		item.Text = cut(item.Text, 1000)
		item.Origins = []models.DiscoveryOrigin{o}
		seen[key] = len(report.Items)
		report.Items = append(report.Items, item)
	}
	queries := discoveryQueries(in)
	for _, item := range known {
		add(item, "collection", queries[0], "collection:"+item.EntityID)
	}
	if len(report.Items) >= 12 {
		return report, nil
	}
	allowed := map[string]bool{}
	for _, seed := range d.policy.Seeds {
		if page, _, _, err := NormalizeEvidenceURL(seed); err == nil {
			u, _ := url.Parse(page)
			allowed[u.Hostname()] = true
		}
	}
	permitted := func(raw string) bool {
		page, _, _, err := NormalizeEvidenceURL(raw)
		if err != nil {
			return false
		}
		u, _ := url.Parse(page)
		return allowed[u.Hostname()]
	}
	visited := map[string]bool{}
	for _, item := range report.Items {
		if item.URL != "" {
			page, _, _, err := NormalizeEvidenceURL(item.URL)
			if err == nil {
				visited[page] = true
			}
		}
	}
	retain := func(page, via string, result models.EvidenceResult) {
		result.Passages = append([]models.EvidencePassage{}, result.Passages...)
		remaining := 3000
		for i := range result.Passages {
			r := []rune(result.Passages[i].Text)
			if len(r) > remaining {
				result.Passages[i].Text = string(r[:remaining])
				result.Passages = result.Passages[:i+1]
				result.Truncated = true
				break
			}
			remaining -= len(r)
		}
		report.Sources = append(report.Sources, models.DiscoverySource{URL: page, Via: via, Result: result})
		text := []string{}
		for _, p := range result.Passages {
			text = append(text, p.Text)
		}
		if result.Outcome == "inspected_page" || result.Outcome == "catalog_only" {
			add(models.DiscoveryItem{Kind: models.CandidateKindMoment, Title: result.Title, Text: strings.Join(text, "\n"), URL: result.FinalURL, Evidence: result.Outcome}, "page", queries[0], via)
		}
	}
	for _, e := range retained {
		if e.Result == nil || !permitted(e.URL) || len(report.Sources) >= 6 {
			continue
		}
		if e.Result.Outcome != "inspected_page" && e.Result.Outcome != "catalog_only" {
			continue
		}
		page, _, _, _ := NormalizeEvidenceURL(e.URL)
		visited[page] = true
		retain(page, "retained:"+e.ID, *e.Result)
	}
	if len(report.Items) >= 12 {
		return report, nil
	}
	type reference struct{ url, via string }
	refs := []reference{}
	inspect := func(raw, via string) error {
		page, _, _, err := NormalizeEvidenceURL(raw)
		if err != nil || !permitted(raw) {
			report.Gaps = append(report.Gaps, "source_outside_policy")
			return nil
		}
		if visited[page] || len(report.Sources) >= 6 {
			return nil
		}
		visited[page] = true
		pageCtx := context.WithValue(ctx, discoveryPageHostsKey{}, allowed)
		result, err := d.registry.client.retrieveEvidence(pageCtx, page, "page", func(final, body string) {
			for _, link := range discoveryLinks(final, body, in) {
				if len(refs) < 12 && permitted(link) {
					refs = append(refs, reference{url: link, via: final})
				}
			}
		})
		if err != nil {
			return err
		}
		retain(page, via, result)
		if result.Outcome != "inspected_page" {
			report.Gaps = append(report.Gaps, "source_"+result.Outcome)
		}
		return nil
	}
	handle := func(err error) bool {
		if errors.Is(err, ErrDiscoveryBudget) {
			report.Gaps = append(report.Gaps, "request_budget")
			return true
		}
		return false
	}
	starts := append(append([]string{}, in.SourceLinks...), d.policy.Seeds...)
	startingPages := 0
	for _, page := range starts {
		if !permitted(page) {
			report.Gaps = append(report.Gaps, "source_outside_policy")
			continue
		}
		normalized, _, _, _ := NormalizeEvidenceURL(page)
		if visited[normalized] {
			continue
		}
		if startingPages == 4 {
			break
		}
		startingPages++
		if err := inspect(page, "starting_page"); err != nil {
			if handle(err) {
				return report, nil
			}
			return report, err
		}
	}
	for i, name := range d.policy.Providers[:min(3, len(d.policy.Providers))] {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		query := queries[i%len(queries)]
		status := models.DiscoveryProvider{Name: name, Query: query, Outcome: "unavailable"}
		adapter, ok := d.registry.Get(name)
		if !ok || !ValidDiscoveryProvider(name) {
			report.Providers = append(report.Providers, status)
			continue
		}
		finds, err := adapter.Discover(ctx, Request{Query: query, Limit: 12, CatalogOnly: true})
		if err != nil {
			report.Providers = append(report.Providers, status)
			if handle(err) {
				return report, nil
			}
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			continue
		}
		status.Outcome = "completed"
		report.Providers = append(report.Providers, status)
		for _, f := range finds[:min(12, len(finds))] {
			evidence := "catalog_only"
			if name == "wikipedia_search" {
				evidence = "search_snippet"
			}
			add(models.DiscoveryItem{Kind: f.Kind, Title: f.Title, Text: f.Text, URL: f.URL, Lat: f.Lat, Lng: f.Lng, Evidence: evidence}, name, query, f.URL)
		}
	}
	// Only the initial reference frontier is eligible: this is not a recursive crawler.
	frontier := append([]reference{}, refs...)
	followed := 0
	for _, ref := range frontier {
		if visited[ref.url] {
			continue
		}
		if followed == 2 {
			break
		}
		followed++
		if err := inspect(ref.url, ref.via); err != nil {
			if handle(err) {
				return report, nil
			}
			return report, err
		}
	}
	return report, nil
}

func discoveryLinks(base, body string, in models.RouteBriefInput) []string {
	root, err := url.Parse(base)
	if err != nil {
		return nil
	}
	z := html.NewTokenizer(strings.NewReader(body))
	out := []string{}
	seen := map[string]bool{}
	for {
		t := z.Next()
		if t == html.ErrorToken {
			break
		}
		if t != html.StartTagToken {
			continue
		}
		token := z.Token()
		if token.Data != "a" {
			continue
		}
		for _, a := range token.Attr {
			if a.Key != "href" {
				continue
			}
			u, err := url.Parse(a.Val)
			if err != nil {
				continue
			}
			u = root.ResolveReference(u)
			page, _, _, err := NormalizeEvidenceURL(u.String())
			if err != nil || seen[page] {
				continue
			}
			// Follow named research links, never nav, sign-in, or every external citation.
			label, _ := url.PathUnescape(u.Path)
			probe := models.DiscoveryItem{Title: label, Area: in.Area}
			if len(DiscoveryReasons(in, probe, nil)) == 0 {
				continue
			}
			seen[page] = true
			out = append(out, page)
			if len(out) == 12 {
				return out
			}
		}
	}
	return out
}
