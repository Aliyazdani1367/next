package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddNamedMigrationContext("000034_phpmyadmin_login_settings.go", up000034PHPMyAdminLoginSettings, emptyDown)
}

func up000034PHPMyAdminLoginSettings(ctx context.Context, tx *sql.Tx) error {
	dialect := activeDialect()
	if err := addColumn(ctx, tx, dialect, "settings", "phpmyadmin_login_mode", "TEXT NOT NULL DEFAULT 'next'", "VARCHAR(16) NOT NULL DEFAULT 'next'"); err != nil {
		return err
	}
	if err := addColumn(ctx, tx, dialect, "settings", "phpmyadmin_username", "TEXT NOT NULL DEFAULT ''", "VARCHAR(255) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumn(ctx, tx, dialect, "settings", "phpmyadmin_password", "TEXT NOT NULL DEFAULT ''", "VARCHAR(1024) NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `
UPDATE settings
SET phpmyadmin_login_mode = COALESCE(NULLIF(phpmyadmin_login_mode, ''), 'next'),
	phpmyadmin_username = COALESCE(phpmyadmin_username, ''),
	phpmyadmin_password = COALESCE(phpmyadmin_password, '')
WHERE id = 1`)
	return err
}
