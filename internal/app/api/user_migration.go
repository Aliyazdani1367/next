package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	backupapp "github.com/aliyazdani1367/next/internal/app/backup"
)

// User migration lets ANY authenticated admin (not just sudo) bring their own
// users over from a backup of a different Next/Rebecca panel installation —
// e.g. a reseller admin moving from their old panel to this one. Unlike
// Service.Import (a full destructive database replace, sudo-only), this reads
// a foreign archive without ever touching it as SQL, extracts one named
// source admin's user rows, and re-creates each one through the normal
// CreateUser business logic under the CALLING admin's own account — so it can
// never see or modify anyone else's users on this panel, and every migrated
// user gets this panel's own credentials/inbounds like any admin-created user.

const userMigrationUploadTTL = 20 * time.Minute

type userMigrationEntry struct {
	path      string
	expiresAt time.Time
}

// userMigrationRegistry stages an uploaded archive between the inspect and
// import calls (the admin picks a source admin from the inspect response
// before committing). Entries and their backing temp files expire on their
// own if the admin never follows up.
type userMigrationRegistry struct {
	mu      sync.Mutex
	entries map[string]userMigrationEntry
}

func (r *userMigrationRegistry) stage(path string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evictLocked()
	if r.entries == nil {
		r.entries = map[string]userMigrationEntry{}
	}
	token := randomMigrationToken()
	r.entries[token] = userMigrationEntry{path: path, expiresAt: time.Now().Add(userMigrationUploadTTL)}
	return token
}

func (r *userMigrationRegistry) take(token string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evictLocked()
	entry, ok := r.entries[token]
	if !ok {
		return "", false
	}
	delete(r.entries, token)
	return entry.path, true
}

// evictLocked removes expired entries and deletes their backing files.
// Callers must already hold r.mu.
func (r *userMigrationRegistry) evictLocked() {
	now := time.Now()
	for token, entry := range r.entries {
		if now.After(entry.expiresAt) {
			_ = os.Remove(entry.path)
			delete(r.entries, token)
		}
	}
}

func randomMigrationToken() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func (s *Server) handleUserMigrationInspect(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/settings/user-migration/inspect" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	uploadPath, cleanup, err := saveBackupUpload(w, r)
	if err != nil {
		writeBackupUploadError(w, err)
		return
	}
	admins, err := backupapp.InspectForeignArchive(uploadPath)
	if err != nil {
		cleanup()
		writeBackupError(w, err)
		return
	}
	token := s.userMigration.stage(uploadPath)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":  token,
		"admins": admins,
	})
}

type userMigrationImportRequest struct {
	Token               string `json:"token"`
	SourceAdminUsername string `json:"source_admin_username"`
	ServiceID           int64  `json:"service_id"`
}

type userMigrationSkip struct {
	Username string `json:"username"`
	Reason   string `json:"reason"`
}

func (s *Server) handleUserMigrationImport(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/settings/user-migration/import" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var payload userMigrationImportRequest
	if err := decodeOptionalJSON(r, &payload); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload.Token = strings.TrimSpace(payload.Token)
	payload.SourceAdminUsername = strings.TrimSpace(payload.SourceAdminUsername)
	if payload.Token == "" || payload.SourceAdminUsername == "" || payload.ServiceID <= 0 {
		writeError(w, http.StatusBadRequest, "token, source_admin_username, and service_id are required")
		return
	}
	path, ok := s.userMigration.take(payload.Token)
	if !ok {
		writeError(w, http.StatusGone, "This upload has expired. Upload the backup file again.")
		return
	}
	defer os.Remove(path)

	principal, _ := r.Context().Value(adminContextKey).(adminPrincipal)
	admin := principal.Context.Admin

	foreignUsers, err := backupapp.ExtractForeignUsers(path, payload.SourceAdminUsername)
	if err != nil {
		writeBackupError(w, err)
		return
	}

	skipped := []userMigrationSkip{}
	imported := 0
	for _, foreignUser := range foreignUsers {
		if strings.EqualFold(strings.TrimSpace(foreignUser.Status), "deleted") {
			skipped = append(skipped, userMigrationSkip{Username: foreignUser.Username, Reason: "deleted on the source panel"})
			continue
		}
		raw, buildErr := buildUserMigrationPayload(foreignUser, payload.ServiceID)
		if buildErr != nil {
			skipped = append(skipped, userMigrationSkip{Username: foreignUser.Username, Reason: buildErr.Error()})
			continue
		}
		if _, createErr := s.userService.CreateUser(r.Context(), admin, raw); createErr != nil {
			skipped = append(skipped, userMigrationSkip{Username: foreignUser.Username, Reason: createErr.Error()})
			continue
		}
		imported++
		if strings.EqualFold(strings.TrimSpace(foreignUser.Status), "disabled") {
			_, _ = s.userService.UpdateUser(r.Context(), admin, foreignUser.Username, []byte(`{"status":"disabled"}`))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":    len(foreignUsers),
		"imported": imported,
		"skipped":  skipped,
	})
}

// buildUserMigrationPayload maps an extracted foreign user onto the same JSON
// shape the normal create-user endpoint accepts. service_id always comes from
// the destination panel: a foreign panel's service ids are meaningless here,
// and Next no longer accepts manually supplied proxy/inbound config at all —
// every migrated user's real credentials are derived fresh from the chosen
// destination service, exactly like any admin-created user.
func buildUserMigrationPayload(foreignUser backupapp.ForeignUser, serviceID int64) ([]byte, error) {
	username := strings.TrimSpace(foreignUser.Username)
	if username == "" {
		return nil, errMissingUsername
	}
	status := "active"
	if strings.EqualFold(strings.TrimSpace(foreignUser.Status), "on_hold") {
		status = "on_hold"
	}
	payload := map[string]any{
		"username":   username,
		"service_id": serviceID,
		"status":     status,
	}
	if foreignUser.DataLimit != nil {
		payload["data_limit"] = *foreignUser.DataLimit
	}
	if foreignUser.Expire != nil && *foreignUser.Expire > 0 {
		payload["expire"] = *foreignUser.Expire
	}
	if note := strings.TrimSpace(foreignUser.Note); note != "" {
		payload["note"] = note
	}
	if foreignUser.TelegramID != nil {
		payload["telegram_id"] = strconv.FormatInt(*foreignUser.TelegramID, 10)
	}
	if contact := strings.TrimSpace(foreignUser.ContactNumber); contact != "" {
		payload["contact_number"] = contact
	}
	if foreignUser.OnHoldExpireDuration != nil {
		payload["on_hold_expire_duration"] = *foreignUser.OnHoldExpireDuration
	}
	if foreignUser.IPLimit != nil {
		payload["ip_limit"] = *foreignUser.IPLimit
	}
	if foreignUser.AutoDeleteInDays != nil {
		payload["auto_delete_in_days"] = *foreignUser.AutoDeleteInDays
	}
	if strategy := strings.TrimSpace(foreignUser.DataLimitResetStrategy); strategy != "" {
		payload["data_limit_reset_strategy"] = strategy
	}
	return json.Marshal(payload)
}

var errMissingUsername = migrationError("missing username")

type migrationError string

func (e migrationError) Error() string { return string(e) }

func writeBackupUploadError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) || errors.Is(err, errBackupUploadTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "backup upload is too large")
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
