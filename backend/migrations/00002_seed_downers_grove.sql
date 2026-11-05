-- +goose Up
-- Fixed UUIDs so later seeds and tests can reference these rows.
-- City:   00000000-0000-4000-8000-0000000000c1
-- Topics: ...00t1-00t4  Places: ...00p1-00p4  Moments: ...00m1-00m6  Tour: ...00a1

INSERT INTO cities (id, slug, name, region, country_code, center_lat, center_lng, bbox_min_lat, bbox_min_lng, bbox_max_lat, bbox_max_lng, timezone)
VALUES ('00000000-0000-4000-8000-0000000000c1', 'downers-grove', 'Downers Grove', 'Illinois', 'US',
        41.8089, -88.0112, 41.7600, -88.0600, 41.8500, -87.9700, 'America/Chicago');

INSERT INTO topics (id, city_id, slug, name, description) VALUES
  ('00000000-0000-4000-8000-00000000a0a1', '00000000-0000-4000-8000-0000000000c1', 'rail-and-the-commuter-town', 'Rail and the Commuter Town',
   'How the Chicago, Burlington & Quincy line turned a farm settlement into a railroad suburb, and how the daily commute still shapes downtown.'),
  ('00000000-0000-4000-8000-00000000a0a2', '00000000-0000-4000-8000-0000000000c1', 'downtown-after-dark', 'Downtown After Dark',
   'Theaters, marquees, music, and the nightlife that gave the village a reason to gather after the last train home.'),
  ('00000000-0000-4000-8000-00000000a0a3', '00000000-0000-4000-8000-0000000000c1', 'parks-and-public-life', 'Parks and Public Life',
   'Greens, bandshells, and the civic rituals of a suburb that organised itself around shared outdoor space.'),
  ('00000000-0000-4000-8000-00000000a0a4', '00000000-0000-4000-8000-0000000000c1', 'houses-and-builders', 'Houses and Builders',
   'The settlers, blacksmiths, and architects whose buildings survive, and the stories those buildings carry.');

INSERT INTO places (id, city_id, slug, name, summary, kind, latitude, longitude, arrival_radius_m, address, status) VALUES
  ('00000000-0000-4000-8000-00000000b0b1', '00000000-0000-4000-8000-0000000000c1', 'main-street-station', 'Main Street Station',
   'The downtown commuter rail stop on the BNSF line, successor to the Chicago, Burlington & Quincy depot that put Downers Grove on the map.',
   'station', 41.7959, -88.0092, 50, '5001 Main St, Downers Grove, IL', 'published'),
  ('00000000-0000-4000-8000-00000000b0b2', '00000000-0000-4000-8000-0000000000c1', 'tivoli-theatre', 'Tivoli Theatre',
   'A 1928 movie palace built for sound films, still running first-run pictures under its original marquee.',
   'building', 41.7952, -88.0099, 40, '5021 Highland Ave, Downers Grove, IL', 'published'),
  ('00000000-0000-4000-8000-00000000b0b3', '00000000-0000-4000-8000-0000000000c1', 'fishel-park', 'Fishel Park',
   'A small downtown park with a bandshell, home of the summer concert series and a century of village gatherings.',
   'park', 41.7944, -88.0118, 60, 'Grove St & Curtiss St, Downers Grove, IL', 'published'),
  ('00000000-0000-4000-8000-00000000b0b4', '00000000-0000-4000-8000-0000000000c1', 'downers-grove-museum', 'Downers Grove Museum',
   'The village history campus, anchored by the 1846 Blodgett House and run by the park district.',
   'site', 41.8083, -88.0111, 60, '831 Maple Ave, Downers Grove, IL', 'published');

INSERT INTO moments (id, place_id, slug, title, body, date_start, date_end, date_precision, date_display, status) VALUES
  ('00000000-0000-4000-8000-00000000d0d1', '00000000-0000-4000-8000-00000000b0b1', 'the-burlington-arrives',
   'The Burlington Arrives',
   'In 1864 the Chicago, Burlington & Quincy Railroad laid its line through the settlement that Pierce Downer had founded three decades earlier. The tracks did what the farm never could: they made it possible to live here and work in Chicago. Within a decade the village incorporated, and the land on either side of the depot filled with the stores, churches, and houses that still define downtown.

Stand on the platform and look along the rails. Every commuter train that passes is a repeat of the same bargain the village struck in 1864.',
   '1864-01-01', '1873-12-31', 'year', '1864', 'published'),
  ('00000000-0000-4000-8000-00000000d0d2', '00000000-0000-4000-8000-00000000b0b1', 'morning-platform',
   'Morning Platform',
   'The station is less a monument than a rhythm. Generations of residents have repeated the same choreography here: coffee cups, school bags, winter coats, the glance down the line for the headlight. The BNSF line that carries Metra today is the direct descendant of the Burlington route, and the morning crowd is the clearest surviving evidence of what the railroad made this town into.',
   NULL, NULL, 'circa', 'Present day', 'published'),
  ('00000000-0000-4000-8000-00000000d0d3', '00000000-0000-4000-8000-00000000b0b2', 'built-for-talkies',
   'Built for the Talkies',
   'The Tivoli opened on Christmas Day 1928 as one of the first theaters in the country designed from the start for sound films, arriving only a year after "The Jazz Singer" proved that audiences wanted to hear the movies. Its Renaissance Revival auditorium and street-front marquee were built for a suburb that wanted a downtown worth staying in after dark. The theater has run continuously since, and today it is operated by Classic Cinemas, which has owned it since 1978.',
   '1928-12-25', '1928-12-25', 'day', 'December 25, 1928', 'published'),
  ('00000000-0000-4000-8000-00000000d0d4', '00000000-0000-4000-8000-00000000b0b3', 'summer-concert-lawn',
   'Summer Concert Lawn',
   'Fishel Park is small enough to cross in a minute and large enough to hold the village on a summer evening. The bandshell hosts the park district''s free summer concert series, and the lawn fills with folding chairs, blankets, and children running between songs. Read the park as an outdoor archive: the temporary village that appears whenever music starts is the same civic habit that built the bandshell in the first place.',
   NULL, NULL, 'circa', 'Summer evenings', 'published'),
  ('00000000-0000-4000-8000-00000000d0d5', '00000000-0000-4000-8000-00000000b0b4', 'blodgett-house',
   'The Blodgett House',
   'Israel Blodgett, a blacksmith, built this house in 1846, a generation before the railroad. It is one of the oldest surviving buildings in the village, and family tradition holds that the Blodgetts sheltered freedom seekers on the Underground Railroad. The house was moved to the museum campus in 2008 and restored. A museum building is not only a container for artifacts; it is evidence itself. Look at the room scale, the thresholds, and the wear on the floors.',
   '1846-01-01', '1846-12-31', 'year', '1846', 'published'),
  ('00000000-0000-4000-8000-00000000d0d6', '00000000-0000-4000-8000-00000000b0b4', 'pierce-downer-settles',
   'Pierce Downer Settles the Grove',
   'Pierce Downer arrived from New York in 1832 and claimed land at the junction of two Potawatomi trails, in a grove of trees that gave the settlement its name. The farm he started is the origin point the whole museum campus looks back to. Nothing of his cabin survives, which is exactly why the museum keeps the story.',
   '1832-01-01', '1832-12-31', 'year', '1832', 'in_review');

UPDATE places SET primary_moment_id = '00000000-0000-4000-8000-00000000d0d1' WHERE id = '00000000-0000-4000-8000-00000000b0b1';
UPDATE places SET primary_moment_id = '00000000-0000-4000-8000-00000000d0d3' WHERE id = '00000000-0000-4000-8000-00000000b0b2';
UPDATE places SET primary_moment_id = '00000000-0000-4000-8000-00000000d0d4' WHERE id = '00000000-0000-4000-8000-00000000b0b3';
UPDATE places SET primary_moment_id = '00000000-0000-4000-8000-00000000d0d5' WHERE id = '00000000-0000-4000-8000-00000000b0b4';

INSERT INTO moment_topics (moment_id, topic_id) VALUES
  ('00000000-0000-4000-8000-00000000d0d1', '00000000-0000-4000-8000-00000000a0a1'),
  ('00000000-0000-4000-8000-00000000d0d2', '00000000-0000-4000-8000-00000000a0a1'),
  ('00000000-0000-4000-8000-00000000d0d3', '00000000-0000-4000-8000-00000000a0a2'),
  ('00000000-0000-4000-8000-00000000d0d3', '00000000-0000-4000-8000-00000000a0a1'),
  ('00000000-0000-4000-8000-00000000d0d4', '00000000-0000-4000-8000-00000000a0a3'),
  ('00000000-0000-4000-8000-00000000d0d5', '00000000-0000-4000-8000-00000000a0a4'),
  ('00000000-0000-4000-8000-00000000d0d6', '00000000-0000-4000-8000-00000000a0a4');

INSERT INTO moment_sources (moment_id, position, title, url, publisher, retrieved_at, license, note) VALUES
  ('00000000-0000-4000-8000-00000000d0d1', 1, 'Downers Grove, Illinois', 'https://en.wikipedia.org/wiki/Downers_Grove,_Illinois', 'Wikipedia', '2026-11-04', 'CC-BY-SA-4.0', 'Railroad arrival and 1873 incorporation.'),
  ('00000000-0000-4000-8000-00000000d0d1', 2, 'Downers Grove Main Street station', 'https://en.wikipedia.org/wiki/Downers_Grove_Main_Street_station', 'Wikipedia', '2026-11-04', 'CC-BY-SA-4.0', ''),
  ('00000000-0000-4000-8000-00000000d0d2', 1, 'Downers Grove Main Street station', 'https://en.wikipedia.org/wiki/Downers_Grove_Main_Street_station', 'Wikipedia', '2026-11-04', 'CC-BY-SA-4.0', ''),
  ('00000000-0000-4000-8000-00000000d0d3', 1, 'Tivoli Theatre (Downers Grove, Illinois)', 'https://en.wikipedia.org/wiki/Tivoli_Theatre_(Downers_Grove,_Illinois)', 'Wikipedia', '2026-11-04', 'CC-BY-SA-4.0', 'Opening date and sound-film design.'),
  ('00000000-0000-4000-8000-00000000d0d3', 2, 'Classic Cinemas Tivoli Theatre', 'https://www.classiccinemas.com/theatre/tivoli', 'Classic Cinemas', '2026-11-04', '', 'Current operator.'),
  ('00000000-0000-4000-8000-00000000d0d4', 1, 'Fishel Park', 'https://www.dgparks.org/parks/fishel-park', 'Downers Grove Park District', '2026-11-04', '', 'Bandshell and summer concert series.'),
  ('00000000-0000-4000-8000-00000000d0d5', 1, 'Downers Grove Museum', 'https://www.dgparks.org/downers-grove-museum', 'Downers Grove Park District', '2026-11-04', '', 'Blodgett House history and 2008 move.'),
  ('00000000-0000-4000-8000-00000000d0d5', 2, 'Downers Grove, Illinois', 'https://en.wikipedia.org/wiki/Downers_Grove,_Illinois', 'Wikipedia', '2026-11-04', 'CC-BY-SA-4.0', ''),
  ('00000000-0000-4000-8000-00000000d0d6', 1, 'Downers Grove, Illinois', 'https://en.wikipedia.org/wiki/Downers_Grove,_Illinois', 'Wikipedia', '2026-11-04', 'CC-BY-SA-4.0', 'Pierce Downer, 1832. Trail-junction detail awaits a local archive citation.');

INSERT INTO tours (id, city_id, slug, title, summary, topic_id, modes, status, visibility)
VALUES ('00000000-0000-4000-8000-00000000e0e1', '00000000-0000-4000-8000-0000000000c1', 'downtown-memory-loop',
        'Downtown Memory Loop',
        'A short loop from the station to the theater, the park, and the museum campus: how a railroad stop became a town, and what it does after dark.',
        '00000000-0000-4000-8000-00000000a0a1', ARRAY['walk', 'bike']::travel_mode[], 'published', 'public');

INSERT INTO tour_stops (tour_id, position, place_id, featured_moment_id, note) VALUES
  ('00000000-0000-4000-8000-00000000e0e1', 1, '00000000-0000-4000-8000-00000000b0b1', '00000000-0000-4000-8000-00000000d0d1', 'Start where the town started: the tracks.'),
  ('00000000-0000-4000-8000-00000000e0e1', 2, '00000000-0000-4000-8000-00000000b0b2', '00000000-0000-4000-8000-00000000d0d3', 'The railroad brought people; the theater gave them a reason to stay downtown.'),
  ('00000000-0000-4000-8000-00000000e0e1', 3, '00000000-0000-4000-8000-00000000b0b3', NULL, 'A breather on the lawn before the longer leg north.'),
  ('00000000-0000-4000-8000-00000000e0e1', 4, '00000000-0000-4000-8000-00000000b0b4', '00000000-0000-4000-8000-00000000d0d5', 'End a generation before the railroad, in the blacksmith''s house.');

INSERT INTO tour_routes (tour_id, mode, status) VALUES
  ('00000000-0000-4000-8000-00000000e0e1', 'walk', 'pending'),
  ('00000000-0000-4000-8000-00000000e0e1', 'bike', 'pending');

INSERT INTO jobs (kind, payload) VALUES
  ('compute_route', '{"tour_id":"00000000-0000-4000-8000-00000000e0e1","mode":"walk"}'),
  ('compute_route', '{"tour_id":"00000000-0000-4000-8000-00000000e0e1","mode":"bike"}');

-- +goose Down
DELETE FROM jobs WHERE kind = 'compute_route' AND payload->>'tour_id' = '00000000-0000-4000-8000-00000000e0e1';
DELETE FROM tours WHERE id = '00000000-0000-4000-8000-00000000e0e1';
UPDATE places SET primary_moment_id = NULL WHERE city_id = '00000000-0000-4000-8000-0000000000c1';
DELETE FROM cities WHERE id = '00000000-0000-4000-8000-0000000000c1';
