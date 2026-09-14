package bot

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (b *Bot) handleBackupCommand(ctx context.Context, settings Settings, chatID int64) {
	if b.backup == nil {
		b.reply(ctx, settings, chatID, "Backup is not available.", nil)
		return
	}
	status, err := b.backup.Status(ctx)
	if err != nil {
		b.reply(ctx, settings, chatID, actionErrorText("read backup status", err), nil)
		return
	}
	b.reply(ctx, settings, chatID, backupStatusText(status), backupMenuKeyboard())
}

// handleBackupDocument stages an uploaded backup archive for restore. The
// actual restore only runs after the admin taps the confirm button, so a
// document alone never mutates panel state.
func (b *Bot) handleBackupDocument(ctx context.Context, settings Settings, chatID int64, doc *Document) {
	if b.backup == nil {
		b.reply(ctx, settings, chatID, "Backup is not available.", nil)
		return
	}
	if doc.FileSize > 0 && doc.FileSize > maxTelegramDownloadBytes {
		b.reply(ctx, settings, chatID, fmt.Sprintf(
			"This file is %s, above Telegram's %d MB bot download limit. Restore it from the panel's Settings → Backup page instead.",
			formatBytes(doc.FileSize), maxTelegramDownloadBytes>>20), nil)
		return
	}
	info, err := b.client.getFile(ctx, settings, doc.FileID)
	if err != nil {
		b.reply(ctx, settings, chatID, actionErrorText("fetch the uploaded file", err), nil)
		return
	}
	content, err := b.client.downloadFile(ctx, settings, info.FilePath)
	if err != nil {
		b.reply(ctx, settings, chatID, actionErrorText("download the uploaded file", err), nil)
		return
	}
	path, err := stageRestoreUpload(content, doc.FileName)
	if err != nil {
		b.reply(ctx, settings, chatID, actionErrorText("stage the uploaded file", err), nil)
		return
	}
	if err := b.state.set(ctx, chatID, stateAwaitRestoreConfirm, path); err != nil {
		_ = os.Remove(path)
		b.reply(ctx, settings, chatID, actionErrorText("stage the uploaded file", err), nil)
		return
	}
	name := strings.TrimSpace(doc.FileName)
	if name == "" {
		name = "backup archive"
	}
	b.reply(ctx, settings, chatID, fmt.Sprintf(
		"⚠️ <b>Restore %s?</b>\n\nThis replaces the panel's ENTIRE database and every user with the contents of this file. "+
			"A safety copy of the current state is taken automatically first, and it's restored back if the import fails — but a bad file can still wipe recent changes.\n\n"+
			"Confirm within a few minutes or send /help to cancel.",
		escape(name)), restoreConfirmKeyboard())
}

func (b *Bot) handleRestoreCallback(ctx context.Context, settings Settings, query *CallbackQuery, chatID, messageID int64, confirmed bool) {
	conv, ok := b.state.get(ctx, chatID)
	if !ok || conv.State != stateAwaitRestoreConfirm {
		_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Nothing pending")
		return
	}
	_ = b.state.clear(ctx, chatID)
	path := conv.Payload
	defer os.Remove(path)

	if !confirmed {
		_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Cancelled")
		_ = b.client.editMessageText(ctx, settings, chatID, messageID, "❌ Restore cancelled. No changes were made.", nil)
		return
	}
	_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Restoring…")
	_ = b.client.editMessageText(ctx, settings, chatID, messageID, "⏳ Restoring backup, this can take a moment…", nil)

	result, err := b.backup.Restore(ctx, path)
	if err != nil {
		_ = b.client.editMessageText(ctx, settings, chatID, messageID, actionErrorText("restore the backup", err), nil)
		return
	}
	_ = b.client.editMessageText(ctx, settings, chatID, messageID, backupRestoreResultText(result), nil)
}

func (b *Bot) handleBackupSetSchedule(ctx context.Context, settings Settings, query *CallbackQuery, chatID, messageID int64, arg string) {
	value, unit, err := parseSchedulePreset(arg)
	if err != nil {
		_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Invalid preset")
		return
	}
	if err := b.backup.SetSchedule(ctx, value, unit); err != nil {
		_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Failed")
		_ = b.client.editMessageText(ctx, settings, chatID, messageID, actionErrorText("update the backup schedule", err), nil)
		return
	}
	_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Schedule updated")
	status, err := b.backup.Status(ctx)
	if err != nil {
		_ = b.client.editMessageText(ctx, settings, chatID, messageID, fmt.Sprintf("✅ Backup scheduled every %d %s.", value, unit), backupMenuKeyboard())
		return
	}
	_ = b.client.editMessageText(ctx, settings, chatID, messageID, "✅ Schedule updated.\n\n"+backupStatusText(status), backupMenuKeyboard())
}

func (b *Bot) handleBackupSendNow(ctx context.Context, settings Settings, query *CallbackQuery, chatID, messageID int64) {
	_ = b.client.answerCallbackQuery(ctx, settings, query.ID, "Sending…")
	result, err := b.backup.SendNow(ctx)
	if err != nil {
		_ = b.client.editMessageText(ctx, settings, chatID, messageID, actionErrorText("send the backup", err), backupMenuKeyboard())
		return
	}
	_ = b.client.editMessageText(ctx, settings, chatID, messageID,
		fmt.Sprintf("✅ Backup sent: <code>%s</code> (%s).", escape(result.Filename), formatBytes(result.Size)), backupMenuKeyboard())
}

func parseSchedulePreset(arg string) (int, string, error) {
	parts := strings.SplitN(arg, ":", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid preset")
	}
	value, err := strconv.Atoi(parts[0])
	if err != nil || value <= 0 {
		return 0, "", fmt.Errorf("invalid preset")
	}
	unit := parts[1]
	if unit != "hours" && unit != "days" && unit != "minutes" {
		return 0, "", fmt.Errorf("invalid preset")
	}
	return value, unit, nil
}

func stageRestoreUpload(content []byte, filename string) (string, error) {
	suffix := ".rbbackup"
	if trimmed := strings.TrimSpace(filename); trimmed != "" {
		if idx := strings.LastIndex(trimmed, "."); idx >= 0 {
			suffix = trimmed[idx:]
		}
	}
	f, err := os.CreateTemp("", "next-bot-restore-*"+suffix)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func backupStatusText(status BackupStatus) string {
	state := "disabled"
	if status.Enabled {
		state = fmt.Sprintf("every %d %s", status.IntervalValue, status.IntervalUnit)
	}
	lines := []string{
		"📦 <b>Periodic backup</b>",
		line("Schedule", state),
		line("Scope", firstNonEmpty(status.Scope, "database")),
		line("Last sent", formatOptionalTime(status.LastSentAt)),
	}
	if status.LastError != nil && strings.TrimSpace(*status.LastError) != "" {
		lines = append(lines, line("Last error", *status.LastError))
	}
	lines = append(lines, "", "Pick a schedule below, or send a backup file here to restore it.")
	return strings.Join(lines, "\n")
}

func backupRestoreResultText(result BackupRestoreResult) string {
	lines := []string{
		"✅ <b>Restore complete</b>",
		line("Tables restored", strconv.Itoa(result.TablesRestored)),
		line("Rows restored", strconv.Itoa(result.RowsRestored)),
	}
	if len(result.FilesRestored) > 0 {
		lines = append(lines, line("Files restored", strconv.Itoa(len(result.FilesRestored))))
	}
	for _, warning := range result.Warnings {
		lines = append(lines, "⚠️ "+escape(warning))
	}
	if strings.TrimSpace(result.SafetyBackupPath) != "" {
		lines = append(lines, "", "A safety copy of the previous state was saved on the server at:", "<code>"+escape(result.SafetyBackupPath)+"</code>")
	}
	return strings.Join(lines, "\n")
}

func formatOptionalTime(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "never"
	}
	return *value
}
