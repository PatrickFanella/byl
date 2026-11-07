-- +goose Up
-- Keep each topic's original city/slug reference; additional municipalities share its ID.
CREATE TABLE topic_cities (
  topic_id UUID NOT NULL REFERENCES topics(id) ON DELETE CASCADE,
  city_id UUID NOT NULL REFERENCES cities(id) ON DELETE CASCADE,
  PRIMARY KEY (topic_id, city_id)
);
CREATE INDEX topic_cities_city_idx ON topic_cities (city_id, topic_id);

-- Preserve existing associations, including references written by earlier APIs.
INSERT INTO topic_cities (topic_id, city_id)
SELECT id, city_id FROM topics
UNION
SELECT mt.topic_id, p.city_id FROM moment_topics mt
JOIN moments m ON m.id=mt.moment_id JOIN places p ON p.id=m.place_id
UNION
SELECT topic_id, city_id FROM tours WHERE topic_id IS NOT NULL
UNION
SELECT t.topic_id, p.city_id FROM tours t JOIN tour_stops ts ON ts.tour_id=t.id
JOIN places p ON p.id=ts.place_id WHERE t.topic_id IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
  RAISE EXCEPTION 'Regional topic associations may contain curator work; use image rollback and retain this additive schema';
END $$;
-- +goose StatementEnd
