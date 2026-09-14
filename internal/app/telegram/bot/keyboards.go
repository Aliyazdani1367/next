package bot

import (
	"fmt"
	"strings"
)

// Callback data prefixes for the user detail inline keyboard. They mirror the
// legacy Python callbacks (delete/suspend/activate/reset_usage/revoke_sub/...).
const (
	cbActivate   = "activate:"
	cbSuspend    = "suspend:"
	cbResetUsage = "reset_usage:"
	cbRevokeSub  = "revoke_sub:"
	cbDelete     = "delete:"
	cbDeleteYes  = "delete_yes:"
	cbLinks      = "links:"
	cbEditNote   = "edit_note:"
	cbRefresh    = "refresh:"

	cbBackupSetSchedule = "backup_set:"
	cbBackupSendNow     = "backup_send"
	cbRestoreConfirm    = "restore_confirm"
	cbRestoreCancel     = "restore_cancel"
)

// backupSchedulePresets are the interval choices offered on the /backup menu.
// Keeping this to fixed presets (rather than free-text chat input) avoids
// parsing/validating arbitrary admin-typed intervals for a scheduler that
// pushes the entire panel database.
var backupSchedulePresets = []struct {
	Label string
	Value int
	Unit  string
}{
	{"Every 6 hours", 6, "hours"},
	{"Every 12 hours", 12, "hours"},
	{"Every 24 hours", 24, "hours"},
	{"Every 3 days", 3, "days"},
	{"Every 7 days", 7, "days"},
}

func backupMenuKeyboard() *InlineKeyboard {
	rows := make([][]InlineButton, 0, len(backupSchedulePresets)+1)
	for _, preset := range backupSchedulePresets {
		rows = append(rows, []InlineButton{{
			Text:         preset.Label,
			CallbackData: fmt.Sprintf("%s%d:%s", cbBackupSetSchedule, preset.Value, preset.Unit),
		}})
	}
	rows = append(rows, []InlineButton{{Text: "📤 Send backup now", CallbackData: cbBackupSendNow}})
	return &InlineKeyboard{InlineKeyboard: rows}
}

func restoreConfirmKeyboard() *InlineKeyboard {
	return &InlineKeyboard{InlineKeyboard: [][]InlineButton{
		{{Text: "✅ Confirm restore", CallbackData: cbRestoreConfirm}, {Text: "✖️ Cancel", CallbackData: cbRestoreCancel}},
	}}
}

func mainMenuKeyboard() *InlineKeyboard {
	return &InlineKeyboard{InlineKeyboard: [][]InlineButton{
		{{Text: "🖥 System", CallbackData: "system"}},
	}}
}

// userMenuKeyboard builds the lifecycle keyboard for a user, choosing between
// activate/suspend depending on the current status.
func userMenuKeyboard(username string, status string) *InlineKeyboard {
	toggle := InlineButton{Text: "⛔ Disable", CallbackData: cbSuspend + username}
	if strings.EqualFold(strings.TrimSpace(status), "disabled") {
		toggle = InlineButton{Text: "✅ Activate", CallbackData: cbActivate + username}
	}
	return &InlineKeyboard{InlineKeyboard: [][]InlineButton{
		{toggle, {Text: "🔄 Reset usage", CallbackData: cbResetUsage + username}},
		{{Text: "🚫 Revoke sub", CallbackData: cbRevokeSub + username}, {Text: "🔗 Links", CallbackData: cbLinks + username}},
		{{Text: "📝 Edit note", CallbackData: cbEditNote + username}, {Text: "♻️ Refresh", CallbackData: cbRefresh + username}},
		{{Text: "🗑 Delete", CallbackData: cbDelete + username}},
	}}
}

func confirmDeleteKeyboard(username string) *InlineKeyboard {
	return &InlineKeyboard{InlineKeyboard: [][]InlineButton{
		{{Text: "✅ Yes, delete", CallbackData: cbDeleteYes + username}, {Text: "✖️ Cancel", CallbackData: cbRefresh + username}},
	}}
}

// parseCallback splits "prefix:value" callback data into the prefix (with colon)
// and the value.
func parseCallback(data string) (prefix string, value string) {
	idx := strings.Index(data, ":")
	if idx < 0 {
		return data, ""
	}
	return data[:idx+1], data[idx+1:]
}
