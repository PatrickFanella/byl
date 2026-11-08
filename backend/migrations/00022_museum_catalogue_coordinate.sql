-- +goose Up
-- Inspected 2026-11-08: https://www.dgparks.org/downers-grove-museum
-- 831 Maple Avenue; OSM building way 684600474 / Nominatim named address.
-- Independent campus reference: https://www.hmdb.org/m.asp?m=270861
-- Physical-site position only: no visitor-arrival or current-access approval.
WITH corrected AS (
 UPDATE places SET latitude=41.7923663, longitude=-88.0045442, updated_at=now()
 WHERE id='00000000-0000-4000-8000-00000000b0b4'
   AND slug='downers-grove-museum'
   AND address='831 Maple Ave, Downers Grove, IL'
   AND latitude=41.8083 AND longitude=-88.0111
 RETURNING id
)
UPDATE tour_revision_state SET draft_version=draft_version+1,
 release_requested_version=NULL
WHERE tour_id IN (SELECT ts.tour_id FROM tour_stops ts JOIN corrected c ON c.id=ts.place_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 RAISE EXCEPTION 'Retain the inspected catalogue coordinate; image rollback does not restore the known wrong seed pin';
END $$;
-- +goose StatementEnd
