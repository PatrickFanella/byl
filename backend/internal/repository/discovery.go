package repository

import (
	"context"
	"strings"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
)

// DiscoveryCollection selects relevant published collection records before any external request.
// A city is resolved only by its complete name (optionally with its configured region).
func (s *Store) DiscoveryCollection(ctx context.Context, in models.RouteBriefInput) ([]models.DiscoveryItem, *models.DiscoveryScope, error) {
	cities, err := s.ListCities(ctx)
	if err != nil {
		return nil, nil, err
	}
	var scope *models.DiscoveryScope
	area := sources.DiscoveryTerms(in.Area)
	for _, city := range cities {
		if area == sources.DiscoveryTerms(city.Name) || area == sources.DiscoveryTerms(city.Name+" "+city.Region) {
			if scope != nil {
				scope = nil
				break
			} // ambiguous names require explicit source geography
			scope = &models.DiscoveryScope{CityID: city.ID, BBox: city.BBox}
		}
	}
	cityID := ""
	if scope != nil {
		cityID = scope.CityID
	}
	terms := []string{}
	for _, w := range strings.Fields(sources.DiscoveryTerms(in.Topic + " " + strings.Join(in.PlaceLeads, " ") + " " + strings.Join(in.EventLeads, " "))) {
		if len(w) >= 3 && !strings.Contains(" "+area+" ", " "+w+" ") {
			terms = append(terms, w)
		}
	}
	if len(terms) > 32 {
		terms = terms[:32]
	}
	rows, err := s.pool.Query(ctx, `WITH collection AS (
	 SELECT 'place' AS kind,p.id::text AS entity_id,p.name AS title,p.summary AS body,p.latitude,p.longitude,c.id::text AS city_id,c.name || ' ' || c.region AS area
	 FROM places p JOIN cities c ON c.id=p.city_id WHERE p.status='published'
	 UNION ALL
	 SELECT 'moment',m.id::text,m.title,m.body,p.latitude,p.longitude,c.id::text,c.name || ' ' || c.region
	 FROM moments m JOIN places p ON p.id=m.place_id JOIN cities c ON c.id=p.city_id WHERE m.status='published' AND p.status='published'
	) SELECT kind,entity_id,title,left(body,20000),latitude,longitude,area FROM collection
	 WHERE (($1<>'' AND city_id=$1) OR ($1='' AND strpos(regexp_replace(lower(area || ' ' || title || ' ' || body),'[^[:alnum:]]+',' ','g'),$2)>0))
	 AND EXISTS (SELECT 1 FROM unnest($3::text[]) word WHERE strpos(lower(title || ' ' || body),word)>0)
	 ORDER BY entity_id LIMIT 100`, cityID, area, terms)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []models.DiscoveryItem{}
	for rows.Next() {
		var item models.DiscoveryItem
		if err := rows.Scan(&item.Kind, &item.EntityID, &item.Title, &item.Text, &item.Lat, &item.Lng, &item.Area); err != nil {
			return nil, nil, err
		}
		item.Evidence = "published_collection"
		out = append(out, item)
	}
	return out, scope, rows.Err()
}

// ReserveDiscoveryRequest charges an additional HTTP hop under the existing run
// and worker fence. No model tokens are spent by archive or page requests. A denied
// reservation leaves the stage active so its successful partial results can commit.
func (s *Store) ReserveDiscoveryRequest(ctx context.Context, runID string, claim *models.Job, prepaid bool) (bool, error) {
	allowed := false
	err := s.Transaction(ctx, func(tx *Store) error {
		r, b, err := tx.lockResearch(ctx, runID, claim)
		if err != nil {
			return err
		}
		if r.Status != "active" || b.Version != r.BriefVersion || b.Status != models.BriefProcessing {
			return ErrResearchInactive
		}
		if time.Now().After(r.DeadlineAt) || (!prepaid && r.RequestsUsed >= r.RequestLimit) {
			return nil
		}
		var running bool
		if err := tx.pool.QueryRow(ctx, `SELECT status='running' FROM brief_research_stages WHERE run_id=$1 AND stage='research'`, runID).Scan(&running); err != nil {
			return err
		}
		if !running {
			return ErrResearchInactive
		}
		if prepaid {
			allowed = true
			return nil
		}
		_, err = tx.pool.Exec(ctx, `UPDATE brief_research_runs SET requests_used=requests_used+1,updated_at=now() WHERE id=$1`, runID)
		allowed = err == nil
		return err
	})
	return allowed, err
}
