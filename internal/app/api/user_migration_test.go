//go:build cgo

package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	adminapp "github.com/aliking1367/next/internal/app/admin"
	backupapp "github.com/aliking1367/next/internal/app/backup"
)

// buildTestArchiveBytes mirrors the on-disk layout backupapp.Export produces
// (a gzip'd tar with manifest.json plus named payload files), entirely in
// memory, so tests can upload a synthetic foreign-panel backup. format/version
// are read by InspectForeignArchive/ExtractForeignUsers from manifest.json;
// backupapp itself does not export the manifest struct, so this writes the
// same JSON shape by hand.
type testManifest struct {
	Format  string `json:"format"`
	Version int    `json:"version"`
	Scope   string `json:"scope"`
}

func buildTestArchiveBytes(t *testing.T, format string, version int, scope string, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	manifestBytes, err := json.Marshal(testManifest{Format: format, Version: version, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	writeTestTarFile(t, tw, backupapp.ManifestName, manifestBytes)
	for name, content := range files {
		writeTestTarFile(t, tw, name, []byte(content))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTestTarFile(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
}

func TestBuildUserMigrationPayloadMapsFields(t *testing.T) {
	dataLimit := int64(1000)
	expire := int64(2000000000)
	telegramID := int64(555)

	raw, err := buildUserMigrationPayload(backupapp.ForeignUser{
		Username:      "alice",
		Status:        "on_hold",
		DataLimit:     &dataLimit,
		Expire:        &expire,
		Note:          "vip",
		TelegramID:    &telegramID,
		ContactNumber: "0912",
	}, 7)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["username"] != "alice" || decoded["status"] != "on_hold" {
		t.Fatalf("unexpected payload: %v", decoded)
	}
	if int64(decoded["service_id"].(float64)) != 7 {
		t.Fatalf("expected service_id 7, got %v", decoded["service_id"])
	}
	if int64(decoded["data_limit"].(float64)) != 1000 {
		t.Fatalf("expected data_limit 1000, got %v", decoded["data_limit"])
	}
	if decoded["telegram_id"] != "555" {
		t.Fatalf("expected telegram_id as string \"555\", got %v", decoded["telegram_id"])
	}
	if decoded["note"] != "vip" {
		t.Fatalf("expected note vip, got %v", decoded["note"])
	}

	// Active status and empty optional fields must be omitted, not sent as zero values.
	raw, err = buildUserMigrationPayload(backupapp.ForeignUser{Username: "bob", Status: "disabled"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	decoded = map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["status"] != "active" {
		t.Fatalf("expected non on_hold/active source status to create as active, got %v", decoded["status"])
	}
	for _, key := range []string{"data_limit", "expire", "note", "telegram_id", "contact_number"} {
		if _, present := decoded[key]; present {
			t.Fatalf("expected %q to be omitted, got %v", key, decoded[key])
		}
	}
}

func TestBuildUserMigrationPayloadRejectsBlankUsername(t *testing.T) {
	if _, err := buildUserMigrationPayload(backupapp.ForeignUser{Username: "  "}, 1); err == nil {
		t.Fatal("expected an error for a blank username")
	}
}

func TestUserMigrationRegistryStageTakeAndExpiry(t *testing.T) {
	var reg userMigrationRegistry
	token := reg.stage("/tmp/does-not-matter")
	if token == "" {
		t.Fatal("expected a non-empty token")
	}
	path, ok := reg.take(token)
	if !ok || path != "/tmp/does-not-matter" {
		t.Fatalf("take() = %q, %v", path, ok)
	}
	if _, ok := reg.take(token); ok {
		t.Fatal("token must not be reusable after being taken")
	}

	reg.mu.Lock()
	reg.entries = map[string]userMigrationEntry{
		"expired": {path: "/tmp/expired", expiresAt: time.Now().Add(-time.Minute)},
	}
	reg.mu.Unlock()
	if _, ok := reg.take("expired"); ok {
		t.Fatal("expired entries must not be returned")
	}
}

func legacyJSONArchiveFixture(t *testing.T) []byte {
	t.Helper()
	dbJSON := `{
		"format": "next-backup",
		"version": 1,
		"tables": [
			{"name": "admins", "columns": ["id","username"], "rows": [
				{"id": 1, "username": "reseller_a"},
				{"id": 2, "username": "reseller_b"}
			]},
			{"name": "users", "columns": ["username","status","admin_id"], "rows": [
				{"username": "alice", "status": "active", "admin_id": 1},
				{"username": "bob", "status": "active", "admin_id": 1},
				{"username": "carol", "status": "active", "admin_id": 2}
			]}
		]
	}`
	return buildTestArchiveBytes(t, backupapp.Format, backupapp.Version, backupapp.ScopeDatabase, map[string]string{
		backupapp.DatabaseDumpName: dbJSON,
	})
}

func multipartBackupUpload(t *testing.T, server *Server, path string, token string, content []byte, filename string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	return rec
}

func TestUserMigrationInspectListsAdminsFromForeignArchive(t *testing.T) {
	server, db := testAdminServer(t)
	insertMasterAPIAdmin(t, db, 1, "owner", "pass123", adminapp.RoleFullAccess, adminapp.StatusActive)
	token := adminBearerToken(t, server, "owner", "pass123")

	rec := multipartBackupUpload(t, server, "/api/settings/user-migration/inspect", token, legacyJSONArchiveFixture(t), "other-panel.rbbackup")
	if rec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token  string                  `json:"token"`
		Admins []backupapp.ForeignAdmin `json:"admins"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Fatal("expected a non-empty token")
	}
	counts := map[string]int{}
	for _, admin := range resp.Admins {
		counts[admin.Username] = admin.UserCount
	}
	if counts["reseller_a"] != 2 || counts["reseller_b"] != 1 {
		t.Fatalf("unexpected admin counts: %+v", resp.Admins)
	}

	// The staged upload must still exist so a follow-up import call can use it.
	path, ok := server.userMigration.take(resp.Token)
	if !ok {
		t.Fatal("expected the inspected upload to remain staged")
	}
	_ = path
}

func TestUserMigrationInspectRejectsUnauthenticated(t *testing.T) {
	server, _ := testAdminServer(t)
	rec := multipartBackupUpload(t, server, "/api/settings/user-migration/inspect", "", legacyJSONArchiveFixture(t), "x.rbbackup")
	if rec.Code == http.StatusOK {
		t.Fatalf("expected an auth failure, got 200: %s", rec.Body.String())
	}
}

func TestUserMigrationImportValidatesRequest(t *testing.T) {
	server, db := testAdminServer(t)
	insertMasterAPIAdmin(t, db, 1, "owner", "pass123", adminapp.RoleFullAccess, adminapp.StatusActive)
	token := adminBearerToken(t, server, "owner", "pass123")

	rec := adminJSONRequest(t, server, http.MethodPost, "/api/settings/user-migration/import", token, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing fields, got %d body=%s", rec.Code, rec.Body.String())
	}

	rec = adminJSONRequest(t, server, http.MethodPost, "/api/settings/user-migration/import", token,
		`{"token":"does-not-exist","source_admin_username":"someone","service_id":1}`)
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 for an unknown/expired token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUserMigrationImportRejectsUnknownSourceAdmin(t *testing.T) {
	server, db := testAdminServer(t)
	insertMasterAPIAdmin(t, db, 1, "owner", "pass123", adminapp.RoleFullAccess, adminapp.StatusActive)
	token := adminBearerToken(t, server, "owner", "pass123")

	inspectRec := multipartBackupUpload(t, server, "/api/settings/user-migration/inspect", token, legacyJSONArchiveFixture(t), "x.rbbackup")
	if inspectRec.Code != http.StatusOK {
		t.Fatalf("inspect status = %d body=%s", inspectRec.Code, inspectRec.Body.String())
	}
	var inspectResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(inspectRec.Body.Bytes(), &inspectResp); err != nil {
		t.Fatal(err)
	}

	rec := adminJSONRequest(t, server, http.MethodPost, "/api/settings/user-migration/import", token,
		`{"token":"`+inspectResp.Token+`","source_admin_username":"ghost","service_id":1}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an admin absent from the archive, got %d body=%s", rec.Code, rec.Body.String())
	}
}
