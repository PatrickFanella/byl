-- +goose Up
-- One SQL statement captures a consistent source view, including reviewed text.
-- +goose StatementBegin
CREATE FUNCTION byl_tour_snapshot(tour UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 WITH stops AS (
  SELECT ts.*,p.primary_moment_id,
   to_jsonb(p) || jsonb_build_object('moment_count',(SELECT count(*) FROM moments WHERE place_id=p.id AND status='published'),
    'arrival_version',COALESCE(ac.version,0),'arrival_point',CASE WHEN ap.id IS NOT NULL THEN jsonb_build_object('id',ap.id,'version',ac.version,'point',ap.point,'reviewed_at',ap.reviewed_at) END) AS place,
   COALESCE((SELECT id FROM moments WHERE id=ts.featured_moment_id AND status='published'),
            (SELECT id FROM moments WHERE id=p.primary_moment_id AND status='published')) AS selected_moment
  FROM tour_stops ts JOIN places p ON p.id=ts.place_id
  LEFT JOIN place_arrival_current ac ON ac.place_id=p.id
  LEFT JOIN place_arrival_proposals ap ON ap.id=ac.proposal_id AND ap.status='approved'
  WHERE ts.tour_id=tour
 ), stories AS (
  SELECT m.id,to_jsonb(m) || jsonb_build_object(
   'hero_media',(SELECT to_jsonb(a) FROM media_assets a WHERE a.id=m.hero_media_id),
   'narration',(SELECT to_jsonb(a) FROM media_assets a WHERE a.id=m.narration_media_id),
   'sources',COALESCE((SELECT jsonb_agg(to_jsonb(ms) ORDER BY ms.position) FROM moment_sources ms WHERE ms.moment_id=m.id),'[]'::jsonb),
   'media',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY mm.position) FROM moment_media mm JOIN media_assets a ON a.id=mm.media_id WHERE mm.moment_id=m.id AND a.status='published'),'[]'::jsonb),
   'topics',COALESCE((SELECT jsonb_agg(to_jsonb(tp) || jsonb_build_object('city_ids',ARRAY(SELECT city_id FROM topic_cities WHERE topic_id=tp.id ORDER BY city_id)) ORDER BY tp.id) FROM moment_topics mt JOIN topics tp ON tp.id=mt.topic_id WHERE mt.moment_id=m.id),'[]'::jsonb)) AS story
  FROM moments m WHERE m.id IN (SELECT selected_moment FROM stops)
 )
 SELECT jsonb_build_object(
  'tour',to_jsonb(t) || jsonb_build_object(
   'city_ids',ARRAY(SELECT city_id FROM (SELECT t.city_id UNION SELECT p.city_id FROM stops s JOIN places p ON p.id=s.place_id) c ORDER BY city_id),
   'topic',(SELECT to_jsonb(tp) || jsonb_build_object('city_ids',ARRAY(SELECT city_id FROM topic_cities WHERE topic_id=tp.id ORDER BY city_id)) FROM topics tp WHERE tp.id=t.topic_id),
   'hero_media',(SELECT to_jsonb(a) FROM media_assets a WHERE a.id=t.hero_media_id),
   'stop_count',(SELECT count(*) FROM stops),
   'stops',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',s.id,'position',s.position,'place',s.place,'featured_moment_id',s.featured_moment_id,'featured_moment',st.story,'note',s.note) ORDER BY s.position) FROM stops s LEFT JOIN stories st ON st.id=s.selected_moment),'[]'::jsonb)),
  'moments',COALESCE((SELECT jsonb_object_agg(id::text,story) FROM stories),'{}'::jsonb),
  'routes',COALESCE((SELECT jsonb_object_agg(mode::text,to_jsonb(r)) FROM tour_routes r WHERE r.tour_id=t.id),'{}'::jsonb),
  'routing_intent',jsonb_build_object('modes',t.modes,'coordinate_source','historical_site',
   'stops',COALESCE((SELECT jsonb_agg(jsonb_build_object('stop_id',s.id,'place_id',s.place_id,'latitude',s.place->'latitude','longitude',s.place->'longitude','arrival_point',s.place->'arrival_point') ORDER BY s.position) FROM stops s),'[]'::jsonb))
 ) FROM tours t WHERE t.id=tour;
$$;
-- +goose StatementEnd

CREATE TABLE tour_revisions (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 tour_id UUID NOT NULL REFERENCES tours(id) ON DELETE RESTRICT,
 number BIGINT NOT NULL CHECK(number>0),
 draft_version BIGINT NOT NULL CHECK(draft_version>=0),
 snapshot JSONB NOT NULL,
 published_by UUID REFERENCES accounts(id) ON DELETE RESTRICT,
 published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(tour_id,number), UNIQUE(tour_id,id)
);
-- +goose StatementBegin
CREATE FUNCTION retain_tour_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'Published tour revisions are immutable';
END $$;
-- +goose StatementEnd
CREATE TRIGGER retain_tour_revision BEFORE UPDATE OR DELETE ON tour_revisions
 FOR EACH ROW EXECUTE FUNCTION retain_tour_revision();

CREATE TABLE tour_revision_state (
 tour_id UUID PRIMARY KEY REFERENCES tours(id) ON DELETE CASCADE,
 draft_version BIGINT NOT NULL DEFAULT 0,
 route_version BIGINT NOT NULL DEFAULT 0,
 current_revision_id UUID,
 release_requested_version BIGINT,
 deleted_at TIMESTAMPTZ,
 FOREIGN KEY(tour_id,current_revision_id) REFERENCES tour_revisions(tour_id,id)
);
INSERT INTO tour_revisions(tour_id,number,draft_version,snapshot)
 SELECT id,1,0,byl_tour_snapshot(id) FROM tours WHERE status='published';
INSERT INTO tour_revision_state(tour_id,current_revision_id,release_requested_version)
 SELECT t.id,r.id,CASE WHEN t.status='published' AND EXISTS(SELECT 1 FROM tour_routes tr WHERE tr.tour_id=t.id AND tr.status<>'ready') THEN 0 END
 FROM tours t LEFT JOIN tour_revisions r ON r.tour_id=t.id;
-- +goose StatementBegin
CREATE FUNCTION initialize_tour_revision_state() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO tour_revision_state(tour_id,release_requested_version)
 VALUES(NEW.id,CASE WHEN NEW.status='published' AND NEW.owner_account_id IS NOT NULL THEN 0 END);
 RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER initialize_tour_revision_state AFTER INSERT ON tours
 FOR EACH ROW EXECUTE FUNCTION initialize_tour_revision_state();

-- +goose StatementBegin
CREATE FUNCTION byl_route_input_hash(tour UUID,mode travel_mode) RETURNS TEXT LANGUAGE sql STABLE AS $$
 SELECT md5(jsonb_build_object('mode',mode,'stops',COALESCE((
  SELECT jsonb_agg(jsonb_build_object('stop_id',ts.id,'place_id',p.id,'lat',p.latitude,'lng',p.longitude) ORDER BY ts.position)
  FROM tour_stops ts JOIN places p ON p.id=ts.place_id WHERE ts.tour_id=tour),'[]'::jsonb))::text);
$$;
-- +goose StatementEnd
CREATE TABLE tour_route_inputs (
 tour_id UUID NOT NULL,
 mode travel_mode NOT NULL,
 input_hash TEXT NOT NULL,
 PRIMARY KEY(tour_id,mode),
 FOREIGN KEY(tour_id,mode) REFERENCES tour_routes(tour_id,mode) ON DELETE CASCADE
);
INSERT INTO tour_route_inputs(tour_id,mode,input_hash)
 SELECT tour_id,mode,byl_route_input_hash(tour_id,mode) FROM tour_routes WHERE status='ready';

CREATE TABLE tour_revision_progress (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 revision_id UUID NOT NULL REFERENCES tour_revisions(id) ON DELETE RESTRICT,
 mode travel_mode NOT NULL,
 visited_stop_ids UUID[] NOT NULL DEFAULT '{}',
 started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 completed_at TIMESTAMPTZ,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(account_id,revision_id)
);
CREATE TABLE legacy_revision_pins (
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 tour_id UUID NOT NULL REFERENCES tours(id) ON DELETE RESTRICT,
 revision_id UUID NOT NULL,
 PRIMARY KEY(account_id,tour_id),
 FOREIGN KEY(tour_id,revision_id) REFERENCES tour_revisions(tour_id,id)
);
INSERT INTO tour_revision_progress(account_id,revision_id,mode,visited_stop_ids,started_at,completed_at,updated_at)
 SELECT p.account_id,r.id,p.mode,
  ARRAY(SELECT (s->>'id')::uuid FROM jsonb_array_elements(r.snapshot->'tour'->'stops') s WHERE (s->>'position')::integer=ANY(p.visited_positions) ORDER BY s->>'id'),
  p.started_at,p.completed_at,p.updated_at
 FROM tour_progress p JOIN tour_revisions r ON r.tour_id=p.tour_id AND r.number=1;
INSERT INTO legacy_revision_pins(account_id,tour_id,revision_id)
 SELECT p.account_id,p.tour_id,r.id FROM tour_progress p JOIN tour_revisions r ON r.tour_id=p.tour_id AND r.number=1;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain published tour revisions and revision progress during image rollback';
END $$;
-- +goose StatementEnd
