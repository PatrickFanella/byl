-- +goose Up
ALTER TYPE media_kind ADD VALUE IF NOT EXISTS 'document';
ALTER TABLE media_assets ADD COLUMN external BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE place_media (
  place_id UUID NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  media_id UUID NOT NULL REFERENCES media_assets(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  PRIMARY KEY (place_id, media_id),
  UNIQUE (place_id, position)
);
CREATE INDEX place_media_asset_idx ON place_media(media_id);

-- +goose Down
-- Keep later attachments and the additive enum value during image recovery.
-- +goose StatementBegin
DO $$ BEGIN
  RAISE EXCEPTION 'Restore reviewed media records selectively; automatic down would remove curator attachments';
END $$;
-- +goose StatementEnd
