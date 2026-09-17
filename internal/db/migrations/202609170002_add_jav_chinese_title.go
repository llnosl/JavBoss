package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext(
		"202609170002_add_jav_chinese_title.go",
		addJavChineseTitle,
		irreversibleMigration,
	)
}

func addJavChineseTitle(ctx context.Context, tx *sql.Tx) error {
	if err := addColumnIfMissing(ctx, tx, "jav", "title_zh", `text NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := addColumnIfMissing(ctx, tx, "jav", "title_zh_source_hash", `text NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	return addColumnIfMissing(ctx, tx, "jav", "title_zh_translated_at", "datetime")
}
