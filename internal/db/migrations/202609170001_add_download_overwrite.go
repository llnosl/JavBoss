package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("202609170001_add_download_overwrite.go", addDownloadOverwrite, irreversibleMigration)
}

func addDownloadOverwrite(ctx context.Context, tx *sql.Tx) error {
	if err := addColumnIfMissing(ctx, tx, "download_job", "jav_code", `text NOT NULL DEFAULT ""`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, tx, "download_job", "overwrite_existing", "numeric NOT NULL DEFAULT false"); err != nil {
		return err
	}
	return execDB(ctx, tx, `CREATE INDEX IF NOT EXISTS idx_download_job_jav_code ON download_job(jav_code)`)
}
