package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	backupapp "github.com/aliyazdani1367/next/internal/app/backup"
	"github.com/aliyazdani1367/next/internal/app/logging"
	telegrambot "github.com/aliyazdani1367/next/internal/app/telegram/bot"
)

// botBackupService adapts the panel's backup engine and Telegram delivery to
// the bot's BackupService, so /backup commands reuse the exact same Go
// services (and binary-install-only gate) as the web Settings page.
type botBackupService struct {
	server *Server
}

func (b botBackupService) Status(ctx context.Context) (telegrambot.BackupStatus, error) {
	settings, err := b.server.telegramRepo.Settings(ctx)
	if err != nil {
		return telegrambot.BackupStatus{}, err
	}
	return telegrambot.BackupStatus{
		Enabled:       settings.BackupEnabled,
		Scope:         firstNonEmpty(settings.BackupScope, backupapp.ScopeDatabase),
		IntervalValue: settings.BackupIntervalValue,
		IntervalUnit:  firstNonEmpty(settings.BackupIntervalUnit, "hours"),
		LastSentAt:    settings.BackupLastSentAt,
		LastError:     settings.BackupLastError,
	}, nil
}

// SetSchedule enables periodic backup at the given interval with the "full"
// scope (database + users + panel/admin files), since a chat-driven schedule
// is meant to be the complete "back up everything" flow the user asked for.
func (b botBackupService) SetSchedule(ctx context.Context, value int, unit string) error {
	raw := map[string]json.RawMessage{
		"backup_enabled":        json.RawMessage("true"),
		"backup_scope":          json.RawMessage(`"` + backupapp.ScopeFull + `"`),
		"backup_interval_value": json.RawMessage(fmt.Sprintf("%d", value)),
		"backup_interval_unit":  json.RawMessage(fmt.Sprintf("%q", unit)),
	}
	_, err := b.server.telegramRepo.UpdateSettings(ctx, raw)
	return err
}

func (b botBackupService) SendNow(ctx context.Context) (telegrambot.BackupSendResult, error) {
	if !b.server.isBinaryRuntime() {
		return telegrambot.BackupSendResult{}, errors.New(backupapp.DisabledDetail)
	}
	result, err := b.server.telegramBackupDelivery().Send(ctx, b.server.backup(), "")
	if err != nil {
		return telegrambot.BackupSendResult{}, err
	}
	return telegrambot.BackupSendResult{Filename: result.Filename, Size: result.Size}, nil
}

// Restore imports an uploaded archive, but only after taking a "full" scope
// safety backup of the current state; if the import fails, it automatically
// re-imports that safety copy so a bad upload can't leave the panel worse off
// than a clear rollback failure message.
func (b botBackupService) Restore(ctx context.Context, archivePath string) (telegrambot.BackupRestoreResult, error) {
	if !b.server.isBinaryRuntime() {
		return telegrambot.BackupRestoreResult{}, errors.New(backupapp.DisabledDetail)
	}
	svc := b.server.backup()

	safetyPath, err := takeSafetyBackup(ctx, svc)
	if err != nil {
		return telegrambot.BackupRestoreResult{}, fmt.Errorf("safety backup failed, restore aborted: %w", err)
	}

	result, importErr := svc.Import(ctx, archivePath)
	if importErr != nil {
		if _, rollbackErr := svc.Import(ctx, safetyPath); rollbackErr != nil {
			logging.Warnf(logging.ComponentTelegram, "bot restore: import failed (%v) and rollback also failed (%v); safety copy at %s", importErr, rollbackErr, safetyPath)
			return telegrambot.BackupRestoreResult{}, fmt.Errorf(
				"restore failed (%v) and the automatic rollback also failed (%v) — panel state may be inconsistent; the pre-restore safety copy is at %s",
				importErr, rollbackErr, safetyPath)
		}
		logging.Warnf(logging.ComponentTelegram, "bot restore: import failed (%v), rolled back to safety copy %s", importErr, safetyPath)
		return telegrambot.BackupRestoreResult{}, fmt.Errorf("restore failed and the previous state was rolled back automatically: %w", importErr)
	}

	logging.Infof(logging.ComponentTelegram, "bot restore: imported %s, tables=%d rows=%d, safety copy at %s",
		filepath.Base(archivePath), result.TablesRestored, result.RowsRestored, safetyPath)
	return telegrambot.BackupRestoreResult{
		TablesRestored:   result.TablesRestored,
		RowsRestored:     result.RowsRestored,
		FilesRestored:    result.FilesRestored,
		Warnings:         result.Warnings,
		SafetyBackupPath: safetyPath,
	}, nil
}

// takeSafetyBackup exports the current full state into a durable directory
// that is deliberately NOT under NEXT_DATA_DIR, since the "full" scope backs
// up that entire directory tree — storing safety copies inside it would bundle
// every previous safety copy into each new backup.
func takeSafetyBackup(ctx context.Context, svc *backupapp.Service) (string, error) {
	result, err := svc.Export(ctx, backupapp.ScopeFull)
	if err != nil {
		return "", err
	}
	defer os.Remove(result.Path)

	dir := preRestoreBackupDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405Z"), filepath.Base(firstNonEmpty(result.Filename, "backup"+backupapp.Extension)))
	dest := filepath.Join(dir, name)
	if err := copyFileContents(result.Path, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func preRestoreBackupDir() string {
	if custom := strings.TrimSpace(os.Getenv("NEXT_PRE_RESTORE_BACKUP_DIR")); custom != "" {
		return custom
	}
	dataDir := strings.TrimSpace(os.Getenv("NEXT_DATA_DIR"))
	if dataDir == "" {
		dataDir = "/var/lib/next"
	}
	return filepath.Join(filepath.Dir(dataDir), "next-pre-restore-backups")
}

func copyFileContents(source string, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
