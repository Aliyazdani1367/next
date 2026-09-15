package backup

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// buildTestArchive writes a manifest.json plus the given named payload files
// into a gzip'd tar file at the returned path, mirroring the layout Export
// produces.
func buildTestArchive(t *testing.T, m manifest, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "test.rbbackup")
	out, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	manifestBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	writeTarFile(t, tw, ManifestName, manifestBytes)
	for name, content := range files {
		writeTarFile(t, tw, name, []byte(content))
	}
	return archivePath
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
}

func baseManifest() manifest {
	return manifest{Format: Format, Version: Version, Scope: ScopeDatabase}
}

func sortAdmins(admins []ForeignAdmin) {
	sort.Slice(admins, func(i, j int) bool { return admins[i].Username < admins[j].Username })
}

// --- legacy JSON -------------------------------------------------------

func TestInspectAndExtractLegacyJSON(t *testing.T) {
	dbJSON := `{
		"format": "next-backup",
		"version": 1,
		"tables": [
			{"name": "admins", "columns": ["id","username"], "rows": [
				{"id": 1, "username": "reseller_a"},
				{"id": 2, "username": "reseller_b"}
			]},
			{"name": "users", "columns": ["username","status","data_limit","expire","note","telegram_id","contact_number","admin_id"], "rows": [
				{"username": "alice", "status": "active", "data_limit": 1073741824, "expire": 1999999999, "note": "vip", "telegram_id": 555, "contact_number": "0912", "admin_id": 1},
				{"username": "bob", "status": "on_hold", "data_limit": null, "expire": null, "note": null, "telegram_id": null, "contact_number": null, "admin_id": 1},
				{"username": "carol", "status": "active", "data_limit": 0, "expire": 0, "note": "", "telegram_id": null, "contact_number": null, "admin_id": 2}
			]}
		]
	}`
	m := baseManifest()
	archivePath := buildTestArchive(t, m, map[string]string{DatabaseDumpName: dbJSON})

	admins, err := InspectForeignArchive(archivePath)
	if err != nil {
		t.Fatalf("InspectForeignArchive: %v", err)
	}
	sortAdmins(admins)
	want := []ForeignAdmin{{Username: "reseller_a", UserCount: 2}, {Username: "reseller_b", UserCount: 1}}
	if !reflect.DeepEqual(admins, want) {
		t.Fatalf("admins = %+v, want %+v", admins, want)
	}

	users, err := ExtractForeignUsers(archivePath, "reseller_a")
	if err != nil {
		t.Fatalf("ExtractForeignUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users for reseller_a, got %d: %+v", len(users), users)
	}
	byName := map[string]ForeignUser{}
	for _, u := range users {
		byName[u.Username] = u
	}
	alice, ok := byName["alice"]
	if !ok {
		t.Fatal("expected alice in extracted users")
	}
	if alice.DataLimit == nil || *alice.DataLimit != 1073741824 {
		t.Fatalf("alice.DataLimit = %v, want 1073741824", alice.DataLimit)
	}
	if alice.Note != "vip" || alice.Status != "active" {
		t.Fatalf("alice fields wrong: %+v", alice)
	}
	bob, ok := byName["bob"]
	if !ok {
		t.Fatal("expected bob in extracted users")
	}
	if bob.DataLimit != nil || bob.Status != "on_hold" {
		t.Fatalf("bob fields wrong: %+v", bob)
	}

	users, err = ExtractForeignUsers(archivePath, "reseller_b")
	if err != nil {
		t.Fatalf("ExtractForeignUsers reseller_b: %v", err)
	}
	if len(users) != 1 || users[0].Username != "carol" {
		t.Fatalf("expected only carol for reseller_b, got %+v", users)
	}

	if _, err := ExtractForeignUsers(archivePath, "ghost"); err == nil {
		t.Fatal("expected an error for an admin not present in the archive")
	}
}

// --- SQLite --------------------------------------------------------------

func TestInspectAndExtractSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "source.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE admins (id INTEGER PRIMARY KEY, username TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY, username TEXT, status TEXT, data_limit INTEGER,
		expire INTEGER, note TEXT, telegram_id INTEGER, contact_number TEXT,
		on_hold_expire_duration INTEGER, on_hold_timeout INTEGER, ip_limit INTEGER,
		auto_delete_in_days INTEGER, data_limit_reset_strategy TEXT, admin_id INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admins (id, username) VALUES (1, 'owner')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, status, data_limit, admin_id) VALUES ('dave', 'active', 5000, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	dbBytes, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}

	m := baseManifest()
	m.Database = manifestDB{Payload: DatabaseSQLiteName, PayloadType: "sqlite-file"}
	archivePath := buildTestArchive(t, m, map[string]string{DatabaseSQLiteName: string(dbBytes)})

	admins, err := InspectForeignArchive(archivePath)
	if err != nil {
		t.Fatalf("InspectForeignArchive: %v", err)
	}
	if len(admins) != 1 || admins[0].Username != "owner" || admins[0].UserCount != 1 {
		t.Fatalf("admins = %+v", admins)
	}

	users, err := ExtractForeignUsers(archivePath, "owner")
	if err != nil {
		t.Fatalf("ExtractForeignUsers: %v", err)
	}
	if len(users) != 1 || users[0].Username != "dave" || users[0].DataLimit == nil || *users[0].DataLimit != 5000 {
		t.Fatalf("users = %+v", users)
	}
}

// --- MySQL dump ------------------------------------------------------------

func TestExtractMySQLInsertRowsParsesEscapesAndNulls(t *testing.T) {
	dump := "INSERT INTO `admins` (`id`, `username`) VALUES (1,'owner');\n" +
		"INSERT INTO `users` (`id`,`username`,`status`,`data_limit`,`note`,`admin_id`) VALUES " +
		"(1,'alice','active',1073741824,'it\\'s vip',1)," +
		"(2,'bob','on_hold',NULL,'quote\"\"test',1);\n"

	cols, rows, err := extractMySQLInsertRows(dump, "users")
	if err != nil {
		t.Fatalf("extractMySQLInsertRows: %v", err)
	}
	wantCols := []string{"id", "username", "status", "data_limit", "note", "admin_id"}
	if !reflect.DeepEqual(cols, wantCols) {
		t.Fatalf("cols = %v, want %v", cols, wantCols)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %+v", len(rows), rows)
	}
	first := zip(cols, rows[0])
	if first["username"] != "alice" || first["note"] != "it's vip" {
		t.Fatalf("row 0 = %+v", first)
	}
	second := zip(cols, rows[1])
	if second["data_limit"] != nil {
		t.Fatalf("expected NULL data_limit, got %+v", second["data_limit"])
	}
}

func TestInspectAndExtractMySQLDump(t *testing.T) {
	dump := "-- dump\n" +
		"INSERT INTO `admins` (`id`, `username`) VALUES (1,'reseller_a'),(2,'reseller_b');\n" +
		"INSERT INTO `users` (`username`,`status`,`data_limit`,`admin_id`) VALUES " +
		"('alice','active',2000,1),('eve','active',NULL,2);\n"

	m := baseManifest()
	m.Database = manifestDB{Payload: DatabaseSQLName, PayloadType: "mysql-dump"}
	archivePath := buildTestArchive(t, m, map[string]string{DatabaseSQLName: dump})

	admins, err := InspectForeignArchive(archivePath)
	if err != nil {
		t.Fatalf("InspectForeignArchive: %v", err)
	}
	sortAdmins(admins)
	want := []ForeignAdmin{{Username: "reseller_a", UserCount: 1}, {Username: "reseller_b", UserCount: 1}}
	if !reflect.DeepEqual(admins, want) {
		t.Fatalf("admins = %+v, want %+v", admins, want)
	}

	users, err := ExtractForeignUsers(archivePath, "reseller_b")
	if err != nil {
		t.Fatalf("ExtractForeignUsers: %v", err)
	}
	if len(users) != 1 || users[0].Username != "eve" || users[0].DataLimit != nil {
		t.Fatalf("users = %+v", users)
	}
}

func TestExtractForeignUsersRejectsUnknownFormat(t *testing.T) {
	m := baseManifest()
	m.Database = manifestDB{Payload: "database.exotic", PayloadType: "exotic"}
	archivePath := buildTestArchive(t, m, map[string]string{"database.exotic": "whatever"})

	if _, err := InspectForeignArchive(archivePath); err == nil {
		t.Fatal("expected an error for an unsupported payload type")
	}
}

// --- bare Marzban/PasarGuard-family database files (no Next wrapper) -------

func TestInspectBareMarzbanStyleSQLiteFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "marzban_db.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Mirrors Marzban's actual admins/users columns (no service_id, telegram_id,
	// contact_number, ip_limit, or credential_key on users; on_hold_timeout is
	// a real DATETIME the way Marzban itself stores it, not a Unix integer).
	if _, err := db.Exec(`CREATE TABLE admins (
		id INTEGER PRIMARY KEY, username TEXT, hashed_password TEXT,
		is_sudo BOOLEAN, telegram_id BIGINT, users_usage BIGINT
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE users (
		id INTEGER PRIMARY KEY, username TEXT, status TEXT, used_traffic BIGINT,
		data_limit BIGINT, data_limit_reset_strategy TEXT, expire INTEGER,
		admin_id INTEGER, note TEXT, on_hold_expire_duration BIGINT,
		on_hold_timeout DATETIME, auto_delete_in_days INTEGER
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO admins (id, username, is_sudo) VALUES (1, 'reseller1', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, status, data_limit, expire, admin_id, on_hold_timeout) VALUES
		('marzuser1', 'active', 5368709120, 1999999999, 1, NULL),
		('marzuser2', 'on_hold', NULL, NULL, 1, '2026-01-15 10:30:00.123456')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Uploaded exactly as Marzban would export it: a bare .sqlite3 file with no
	// Next manifest wrapper at all.
	admins, err := InspectForeignArchive(dbPath)
	if err != nil {
		t.Fatalf("InspectForeignArchive on a bare Marzban sqlite file: %v", err)
	}
	if len(admins) != 1 || admins[0].Username != "reseller1" || admins[0].UserCount != 2 {
		t.Fatalf("admins = %+v", admins)
	}

	users, err := ExtractForeignUsers(dbPath, "reseller1")
	if err != nil {
		t.Fatalf("ExtractForeignUsers: %v", err)
	}
	byName := map[string]ForeignUser{}
	for _, u := range users {
		byName[u.Username] = u
	}
	u1, ok := byName["marzuser1"]
	if !ok || u1.DataLimit == nil || *u1.DataLimit != 5368709120 || u1.Expire == nil || *u1.Expire != 1999999999 {
		t.Fatalf("marzuser1 = %+v", u1)
	}
	u2, ok := byName["marzuser2"]
	if !ok || u2.Status != "on_hold" {
		t.Fatalf("marzuser2 = %+v", u2)
	}
	if u2.OnHoldTimeout == nil {
		t.Fatal("expected on_hold_timeout parsed from a DATETIME string into a Unix timestamp")
	}
	wantUnix := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC).Unix()
	if *u2.OnHoldTimeout != wantUnix {
		t.Fatalf("on_hold_timeout = %d, want %d", *u2.OnHoldTimeout, wantUnix)
	}
}

func TestInspectBareMySQLDumpFile(t *testing.T) {
	dump := "-- MySQL dump 10.13\n" +
		"INSERT INTO `admins` (`id`, `username`, `is_sudo`) VALUES (1,'reseller1',0);\n" +
		"INSERT INTO `users` (`username`,`status`,`data_limit`,`admin_id`) VALUES ('u1','active',1000,1);\n"
	path := filepath.Join(t.TempDir(), "marzban_dump.sql")
	if err := os.WriteFile(path, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}

	admins, err := InspectForeignArchive(path)
	if err != nil {
		t.Fatalf("InspectForeignArchive on a bare mysqldump file: %v", err)
	}
	if len(admins) != 1 || admins[0].Username != "reseller1" || admins[0].UserCount != 1 {
		t.Fatalf("admins = %+v", admins)
	}
}

func TestDetectBareForeignDatabaseLeavesGzipToTheNormalPath(t *testing.T) {
	// A real Next .rbbackup (gzip'd tar) must still go through the normal
	// manifest-based path, not be misdetected as a bare file.
	archivePath := buildTestArchive(t, baseManifest(), map[string]string{
		DatabaseDumpName: `{"format":"next-backup","version":1,"tables":[]}`,
	})
	_, ok, err := detectBareForeignDatabase(archivePath)
	if ok || err != nil {
		t.Fatalf("expected a real .rbbackup to be left to the normal path, got ok=%v err=%v", ok, err)
	}
}

// TestExtractMySQLInsertRowsHandlesRealMysqldumpFormat guards against a real
// regression: mysqldump's DEFAULT output (no --complete-insert) omits the
// column list entirely, e.g. "INSERT INTO `admins` VALUES (1,'admin',...);" —
// columns must then come from the table's own CREATE TABLE definition, in
// declaration order, skipping KEY/CONSTRAINT/PRIMARY KEY entries.
func TestExtractMySQLInsertRowsHandlesRealMysqldumpFormat(t *testing.T) {
	dump := "" +
		"DROP TABLE IF EXISTS `admins`;\n" +
		"CREATE TABLE `admins` (\n" +
		"  `id` int NOT NULL AUTO_INCREMENT,\n" +
		"  `username` varchar(34) NOT NULL,\n" +
		"  `hashed_password` varchar(128) DEFAULT NULL,\n" +
		"  `role` enum('full_access','sudo','standard') NOT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  KEY `ix_admins_username` (`username`)\n" +
		") ENGINE=InnoDB;\n" +
		"LOCK TABLES `admins` WRITE;\n" +
		"INSERT INTO `admins` VALUES (1,'admin','$2a$12$hash','full_access'),(2,'ali2','$2a$12$hash2','sudo');\n" +
		"UNLOCK TABLES;\n" +
		"DROP TABLE IF EXISTS `admins_services`;\n" +
		"CREATE TABLE `admins_services` (\n" +
		"  `admin_id` int NOT NULL,\n" +
		"  `service_id` int NOT NULL\n" +
		") ENGINE=InnoDB;\n" +
		"INSERT INTO `admins_services` VALUES (1,5);\n" +
		"DROP TABLE IF EXISTS `users`;\n" +
		"CREATE TABLE `users` (\n" +
		"  `id` int NOT NULL AUTO_INCREMENT,\n" +
		"  `username` varchar(128) NOT NULL,\n" +
		"  `credential_key` varchar(64) DEFAULT NULL,\n" +
		"  `status` enum('active','on_hold','deleted') NOT NULL,\n" +
		"  `data_limit` bigint DEFAULT NULL,\n" +
		"  `admin_id` int DEFAULT NULL,\n" +
		"  PRIMARY KEY (`id`),\n" +
		"  CONSTRAINT `users_ibfk_1` FOREIGN KEY (`admin_id`) REFERENCES `admins` (`id`)\n" +
		") ENGINE=InnoDB;\n" +
		"INSERT INTO `users` VALUES (1,'5XRoer','dd9c66d0','deleted',NULL,1),(2,'alice','abc123','active',1000,1);\n"

	adminCols, adminRows, err := extractMySQLInsertRows(dump, "admins")
	if err != nil {
		t.Fatalf("extractMySQLInsertRows(admins): %v", err)
	}
	wantAdminCols := []string{"id", "username", "hashed_password", "role"}
	if !reflect.DeepEqual(adminCols, wantAdminCols) {
		t.Fatalf("admin columns = %v, want %v (columns must come from CREATE TABLE, not be missing entirely)", adminCols, wantAdminCols)
	}
	if len(adminRows) != 2 {
		t.Fatalf("expected 2 admin rows, got %d", len(adminRows))
	}
	first := zip(adminCols, adminRows[0])
	if first["username"] != "admin" {
		t.Fatalf("admin row 0 = %+v", first)
	}

	userCols, userRows, err := extractMySQLInsertRows(dump, "users")
	if err != nil {
		t.Fatalf("extractMySQLInsertRows(users): %v", err)
	}
	if len(userRows) != 2 {
		t.Fatalf("expected 2 user rows, got %d: cols=%v", len(userRows), userCols)
	}
	second := zip(userCols, userRows[1])
	if second["username"] != "alice" || second["data_limit"] != "1000" {
		t.Fatalf("user row 1 = %+v", second)
	}
}

func TestInspectRealMarzbanStyleHeaderlessMySQLDump(t *testing.T) {
	dump := "" +
		"CREATE TABLE `admins` (`id` int NOT NULL AUTO_INCREMENT, `username` varchar(34) NOT NULL, PRIMARY KEY (`id`)) ENGINE=InnoDB;\n" +
		"INSERT INTO `admins` VALUES (1,'reseller1'),(2,'reseller2');\n" +
		"CREATE TABLE `users` (`id` int NOT NULL AUTO_INCREMENT, `username` varchar(128) NOT NULL, `status` varchar(20) NOT NULL, `admin_id` int DEFAULT NULL, PRIMARY KEY (`id`)) ENGINE=InnoDB;\n" +
		"INSERT INTO `users` VALUES (1,'u1','active',1),(2,'u2','active',1),(3,'u3','active',2);\n"
	path := filepath.Join(t.TempDir(), "headerless.sql")
	if err := os.WriteFile(path, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}

	admins, err := InspectForeignArchive(path)
	if err != nil {
		t.Fatalf("InspectForeignArchive: %v", err)
	}
	sortAdmins(admins)
	want := []ForeignAdmin{{Username: "reseller1", UserCount: 2}, {Username: "reseller2", UserCount: 1}}
	if !reflect.DeepEqual(admins, want) {
		t.Fatalf("admins = %+v, want %+v", admins, want)
	}
}

// --- X-UI / 3x-ui ------------------------------------------------------

func buildXUIDatabase(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "x-ui.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, password TEXT, login_secret TEXT)`,
		`CREATE TABLE inbounds (id INTEGER PRIMARY KEY, remark TEXT, protocol TEXT, settings TEXT)`,
		`CREATE TABLE client_traffics (id INTEGER PRIMARY KEY, inbound_id INTEGER, enable numeric, email TEXT, up INTEGER, down INTEGER, expiry_time INTEGER, total INTEGER)`,
		`INSERT INTO users (id, username, password) VALUES (1, 'admin', 'hash')`,
		`INSERT INTO inbounds (id, remark, protocol, settings) VALUES (1, 'main', 'vless', '{"clients":[
			{"email":"alice","id":"uuid-1","limitIp":2,"tgId":"12345","totalGB":0,"expiryTime":0},
			{"email":"bob","id":"uuid-2","limitIp":0,"tgId":"","totalGB":0,"expiryTime":0}
		]}')`,
		`INSERT INTO client_traffics (inbound_id, enable, email, up, down, expiry_time, total) VALUES
			(1, 1, 'alice', 1000, 2000, 1759329562000, 32212254720),
			(1, 0, 'bob', 0, 0, 0, 0)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

func TestInspectAndExtractXUIDatabase(t *testing.T) {
	path := buildXUIDatabase(t)

	admins, err := InspectForeignArchive(path)
	if err != nil {
		t.Fatalf("InspectForeignArchive: %v", err)
	}
	if len(admins) != 1 || admins[0].Username != "admin" || admins[0].UserCount != 2 {
		t.Fatalf("admins = %+v", admins)
	}

	users, err := ExtractForeignUsers(path, "admin")
	if err != nil {
		t.Fatalf("ExtractForeignUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d: %+v", len(users), users)
	}
	byName := map[string]ForeignUser{}
	for _, u := range users {
		byName[u.Username] = u
	}

	alice, ok := byName["alice"]
	if !ok {
		t.Fatal("expected alice")
	}
	if alice.Status != "active" {
		t.Fatalf("alice.Status = %q, want active", alice.Status)
	}
	if alice.DataLimit == nil || *alice.DataLimit != 32212254720 {
		t.Fatalf("alice.DataLimit = %v, want 32212254720", alice.DataLimit)
	}
	if alice.Expire == nil || *alice.Expire != 1759329562 {
		t.Fatalf("alice.Expire = %v, want 1759329562 (ms converted to seconds)", alice.Expire)
	}
	if alice.IPLimit == nil || *alice.IPLimit != 2 {
		t.Fatalf("alice.IPLimit = %v, want 2 (from inbound settings limitIp)", alice.IPLimit)
	}
	if alice.TelegramID == nil || *alice.TelegramID != 12345 {
		t.Fatalf("alice.TelegramID = %v, want 12345", alice.TelegramID)
	}

	bob, ok := byName["bob"]
	if !ok {
		t.Fatal("expected bob")
	}
	if bob.Status != "disabled" {
		t.Fatalf("bob.Status = %q, want disabled (enable=0)", bob.Status)
	}
	if bob.DataLimit != nil || bob.Expire != nil {
		t.Fatalf("bob should have no limit/expire (0 means unlimited), got %+v", bob)
	}
}

func TestXUIRejectedWhenSchemaUnrecognized(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "unknown.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE something_else (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := InspectForeignArchive(dbPath); err == nil {
		t.Fatal("expected an error for an unrecognized sqlite schema")
	}
}
