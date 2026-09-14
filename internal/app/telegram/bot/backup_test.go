package bot

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "next-bot-restore-test-*.rbbackup")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

type fakeBackup struct {
	mu            sync.Mutex
	status        BackupStatus
	scheduleCalls []string
	sendResult    BackupSendResult
	sendErr       error
	restoreResult BackupRestoreResult
	restoreErr    error
	restorePaths  []string
}

func (f *fakeBackup) Status(context.Context) (BackupStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, nil
}

func (f *fakeBackup) SetSchedule(_ context.Context, value int, unit string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scheduleCalls = append(f.scheduleCalls, fmt.Sprintf("%d:%s", value, unit))
	f.status.Enabled = true
	f.status.IntervalValue = value
	f.status.IntervalUnit = unit
	return nil
}

func (f *fakeBackup) SendNow(context.Context) (BackupSendResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sendResult, f.sendErr
}

func (f *fakeBackup) Restore(_ context.Context, archivePath string) (BackupRestoreResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restorePaths = append(f.restorePaths, archivePath)
	return f.restoreResult, f.restoreErr
}

// newBackupTestBot builds a Bot wired to a fakeBackup, with an httptest server
// that also answers getFile and the file-download endpoint so
// handleBackupDocument can be exercised end-to-end.
func newBackupTestBot(t *testing.T, backup BackupService, fileContent []byte) (*Bot, *[]capturedCall, Settings) {
	t.Helper()
	var (
		mu    sync.Mutex
		calls []capturedCall
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/file/bot") {
			_, _ = w.Write(fileContent)
			return
		}
		parts := strings.Split(r.URL.Path, "/")
		method := parts[len(parts)-1]
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		calls = append(calls, capturedCall{method: method, body: body})
		mu.Unlock()
		if method == "getFile" {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"documents/backup.rbbackup","file_size":5}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	t.Cleanup(srv.Close)

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE bot_conversation_state (
		chat_id INTEGER PRIMARY KEY,
		state VARCHAR(64) NOT NULL,
		payload TEXT NULL,
		updated_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}

	b := New(Options{
		APIBase:    srv.URL,
		Authorizer: fakeAuthorizer{ok: true},
		Users:      newFakeUsers(),
		System:     fakeSystem{},
		Backup:     backup,
		DB:         db,
		Logf:       func(string, ...any) {},
	})
	settings := Settings{Enabled: true, Token: "TOKEN", AdminChatIDs: []int64{100}}
	return b, &calls, settings
}

func TestBackupCommandShowsStatusAndMenu(t *testing.T) {
	backup := &fakeBackup{status: BackupStatus{Enabled: true, Scope: "full", IntervalValue: 24, IntervalUnit: "hours"}}
	b, calls, settings := newBackupTestBot(t, backup, nil)

	b.handleMessage(context.Background(), settings, &Message{Chat: Chat{ID: 100}, Text: "/backup"})

	call, ok := lastCall(*calls, "sendMessage")
	if !ok {
		t.Fatal("expected sendMessage")
	}
	text, _ := call.body["text"].(string)
	if !strings.Contains(text, "every 24 hours") {
		t.Fatalf("expected schedule in status text, got %q", text)
	}
	if _, ok := call.body["reply_markup"]; !ok {
		t.Fatal("expected backup menu keyboard")
	}
}

func TestBackupSetScheduleCallbackUpdatesSettings(t *testing.T) {
	backup := &fakeBackup{}
	b, calls, settings := newBackupTestBot(t, backup, nil)

	b.handleCallback(context.Background(), settings, &CallbackQuery{
		ID:      "c1",
		From:    &User{ID: 100},
		Message: &Message{MessageID: 5, Chat: Chat{ID: 100}},
		Data:    cbBackupSetSchedule + "6:hours",
	})

	if len(backup.scheduleCalls) != 1 || backup.scheduleCalls[0] != "6:hours" {
		t.Fatalf("expected schedule set to 6:hours, got %v", backup.scheduleCalls)
	}
	if _, ok := lastCall(*calls, "editMessageText"); !ok {
		t.Fatal("expected editMessageText confirming the schedule")
	}
}

func TestBackupSendNowCallback(t *testing.T) {
	backup := &fakeBackup{sendResult: BackupSendResult{Filename: "next-backup.rbbackup", Size: 2048}}
	b, calls, settings := newBackupTestBot(t, backup, nil)

	b.handleCallback(context.Background(), settings, &CallbackQuery{
		ID:      "c1",
		From:    &User{ID: 100},
		Message: &Message{MessageID: 5, Chat: Chat{ID: 100}},
		Data:    cbBackupSendNow,
	})

	call, ok := lastCall(*calls, "editMessageText")
	if !ok {
		t.Fatal("expected editMessageText")
	}
	if text, _ := call.body["text"].(string); !strings.Contains(text, "next-backup.rbbackup") {
		t.Fatalf("expected filename in result text, got %q", text)
	}
}

func TestBackupDocumentTooLargeIsRejectedWithoutDownload(t *testing.T) {
	backup := &fakeBackup{}
	b, calls, settings := newBackupTestBot(t, backup, nil)

	b.handleMessage(context.Background(), settings, &Message{
		Chat:     Chat{ID: 100},
		Document: &Document{FileID: "f1", FileName: "huge.rbbackup", FileSize: maxTelegramDownloadBytes + 1},
	})

	if _, ok := lastCall(*calls, "getFile"); ok {
		t.Fatal("oversized document should be rejected before calling getFile")
	}
	call, ok := lastCall(*calls, "sendMessage")
	if !ok {
		t.Fatal("expected a rejection message")
	}
	if text, _ := call.body["text"].(string); !strings.Contains(text, "20 MB") {
		t.Fatalf("expected size-limit explanation, got %q", text)
	}
	if _, ok := b.state.get(context.Background(), 100); ok {
		t.Fatal("no restore state should be staged for a rejected document")
	}
}

func TestBackupDocumentStagesRestoreConfirmation(t *testing.T) {
	backup := &fakeBackup{}
	b, calls, settings := newBackupTestBot(t, backup, []byte("hello"))

	b.handleMessage(context.Background(), settings, &Message{
		Chat:     Chat{ID: 100},
		Document: &Document{FileID: "f1", FileName: "backup.rbbackup", FileSize: 5},
	})

	conv, ok := b.state.get(context.Background(), 100)
	if !ok || conv.State != stateAwaitRestoreConfirm {
		t.Fatalf("expected await_restore_confirm state, got %+v ok=%v", conv, ok)
	}
	content, err := readFile(conv.Payload)
	if err != nil || string(content) != "hello" {
		t.Fatalf("expected staged file with downloaded content, got %q err=%v", content, err)
	}
	call, ok := lastCall(*calls, "sendMessage")
	if !ok {
		t.Fatal("expected a confirmation prompt")
	}
	if text, _ := call.body["text"].(string); !strings.Contains(text, "Restore") {
		t.Fatalf("expected restore warning text, got %q", text)
	}
	if _, ok := call.body["reply_markup"]; !ok {
		t.Fatal("expected confirm/cancel keyboard")
	}
}

func TestRestoreConfirmCallsRestoreAndClearsState(t *testing.T) {
	backup := &fakeBackup{restoreResult: BackupRestoreResult{TablesRestored: 3, RowsRestored: 42, SafetyBackupPath: "/var/lib/next-pre-restore-backups/x.rbbackup"}}
	b, calls, settings := newBackupTestBot(t, backup, nil)
	stagedPath := writeTempFile(t, "staged-content")
	if err := b.state.set(context.Background(), 100, stateAwaitRestoreConfirm, stagedPath); err != nil {
		t.Fatal(err)
	}

	b.handleCallback(context.Background(), settings, &CallbackQuery{
		ID:      "c1",
		From:    &User{ID: 100},
		Message: &Message{MessageID: 5, Chat: Chat{ID: 100}},
		Data:    cbRestoreConfirm,
	})

	if len(backup.restorePaths) != 1 || backup.restorePaths[0] != stagedPath {
		t.Fatalf("expected Restore called with staged path, got %v", backup.restorePaths)
	}
	if _, ok := b.state.get(context.Background(), 100); ok {
		t.Fatal("restore state should be cleared after confirmation")
	}
	if _, err := readFile(stagedPath); err == nil {
		t.Fatal("staged upload file should be removed after restore")
	}
	call, ok := lastCall(*calls, "editMessageText")
	if !ok {
		t.Fatal("expected a result message")
	}
	if text, _ := call.body["text"].(string); !strings.Contains(text, "next-pre-restore-backups") {
		t.Fatalf("expected safety backup path in result, got %q", text)
	}
}

func TestRestoreCancelSkipsRestoreAndClearsState(t *testing.T) {
	backup := &fakeBackup{}
	b, calls, settings := newBackupTestBot(t, backup, nil)
	stagedPath := writeTempFile(t, "staged-content")
	if err := b.state.set(context.Background(), 100, stateAwaitRestoreConfirm, stagedPath); err != nil {
		t.Fatal(err)
	}

	b.handleCallback(context.Background(), settings, &CallbackQuery{
		ID:      "c1",
		From:    &User{ID: 100},
		Message: &Message{MessageID: 5, Chat: Chat{ID: 100}},
		Data:    cbRestoreCancel,
	})

	if len(backup.restorePaths) != 0 {
		t.Fatalf("cancel must not call Restore, got %v", backup.restorePaths)
	}
	if _, ok := b.state.get(context.Background(), 100); ok {
		t.Fatal("restore state should be cleared after cancel")
	}
	call, ok := lastCall(*calls, "editMessageText")
	if !ok {
		t.Fatal("expected a cancellation message")
	}
	if text, _ := call.body["text"].(string); !strings.Contains(text, "cancelled") {
		t.Fatalf("expected cancellation text, got %q", text)
	}
}

func TestRestoreCallbackWithoutPendingStateIsNoOp(t *testing.T) {
	backup := &fakeBackup{}
	b, _, settings := newBackupTestBot(t, backup, nil)

	b.handleCallback(context.Background(), settings, &CallbackQuery{
		ID:      "c1",
		From:    &User{ID: 100},
		Message: &Message{MessageID: 5, Chat: Chat{ID: 100}},
		Data:    cbRestoreConfirm,
	})

	if len(backup.restorePaths) != 0 {
		t.Fatalf("expected no restore without pending state, got %v", backup.restorePaths)
	}
}
