package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("202609150001_add_video_subtitles.go", addVideoSubtitles, irreversibleMigration)
}

func addVideoSubtitles(ctx context.Context, tx *sql.Tx) error {
	if err := addColumnIfMissing(ctx, tx, "video_location", "subtitles_scanned_at", "datetime"); err != nil {
		return err
	}
	return execStatements(ctx, tx,
		`CREATE TABLE IF NOT EXISTS "video_subtitle" (
			id integer PRIMARY KEY AUTOINCREMENT,
			video_location_id integer NOT NULL,
			kind text NOT NULL,
			identity text NOT NULL,
			stream_index integer NOT NULL DEFAULT -1,
			codec text NOT NULL DEFAULT "",
			language text NOT NULL DEFAULT "",
			title text NOT NULL DEFAULT "",
			is_default numeric NOT NULL DEFAULT false,
			is_forced numeric NOT NULL DEFAULT false,
			relative_path text NOT NULL DEFAULT "",
			created_at datetime,
			updated_at datetime,
			CONSTRAINT fk_video_location_subtitles FOREIGN KEY (video_location_id) REFERENCES video_location(id) ON UPDATE CASCADE ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_video_subtitle_video_location_id ON video_subtitle(video_location_id)`,
		`CREATE INDEX IF NOT EXISTS idx_video_subtitle_kind ON video_subtitle(kind)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_video_subtitle_location_identity ON video_subtitle(video_location_id, identity)`,
	)
}
