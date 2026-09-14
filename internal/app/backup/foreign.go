package backup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ForeignAdmin summarizes one admin found in a foreign backup archive's users
// table, so a caller can offer a picker before importing anything.
type ForeignAdmin struct {
	Username  string `json:"username"`
	UserCount int    `json:"user_count"`
}

// ForeignUser is the subset of a foreign users-table row that can be safely
// re-created on another panel through the normal create-user path. Proxy and
// inbound configuration is deliberately not carried over: Next always derives
// per-user credentials from the destination service's own inbounds, and a
// foreign panel's proxy rows would not be meaningful here anyway.
type ForeignUser struct {
	Username               string
	Status                 string
	DataLimit              *int64
	Expire                 *int64
	Note                   string
	TelegramID             *int64
	ContactNumber          string
	OnHoldExpireDuration   *int64
	OnHoldTimeout          *int64
	IPLimit                *int64
	AutoDeleteInDays       *int64
	DataLimitResetStrategy string
}

// foreignDatabase is the generic, read-only view over a foreign archive's
// database payload that both InspectForeignArchive and ExtractForeignUsers
// query. It never touches this panel's own database.
type foreignDatabase interface {
	admins(ctx context.Context) (map[int64]string, error)          // admin id -> username
	users(ctx context.Context, adminIDs map[string]int64) ([]foreignUserRow, error)
	close()
}

type foreignUserRow struct {
	AdminID int64
	User    ForeignUser
}

// InspectForeignArchive opens an arbitrary Next/Rebecca backup archive
// (possibly from a different panel install, possibly an older schema) and
// lists the admins it contains. archivePath must already be a safely staged
// local file; the caller owns that file's lifecycle.
func InspectForeignArchive(archivePath string) ([]ForeignAdmin, error) {
	db, err := openForeignDatabase(archivePath)
	if err != nil {
		return nil, err
	}
	defer db.close()

	ctx := context.Background()
	admins, err := db.admins(ctx)
	if err != nil {
		return nil, err
	}
	adminByUsername := map[string]int64{}
	for id, username := range admins {
		adminByUsername[username] = id
	}
	rows, err := db.users(ctx, adminByUsername)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, row := range rows {
		if username, ok := admins[row.AdminID]; ok {
			counts[username]++
		}
	}
	result := make([]ForeignAdmin, 0, len(counts))
	for username, count := range counts {
		result = append(result, ForeignAdmin{Username: username, UserCount: count})
	}
	return result, nil
}

// ExtractForeignUsers returns every user belonging to sourceAdminUsername
// inside the archive at archivePath.
func ExtractForeignUsers(archivePath string, sourceAdminUsername string) ([]ForeignUser, error) {
	sourceAdminUsername = strings.TrimSpace(sourceAdminUsername)
	if sourceAdminUsername == "" {
		return nil, Error{Message: "A source admin must be selected"}
	}
	db, err := openForeignDatabase(archivePath)
	if err != nil {
		return nil, err
	}
	defer db.close()

	ctx := context.Background()
	admins, err := db.admins(ctx)
	if err != nil {
		return nil, err
	}
	adminByUsername := map[string]int64{sourceAdminUsername: -1}
	for id, username := range admins {
		if username == sourceAdminUsername {
			adminByUsername[username] = id
		}
	}
	targetID, ok := adminByUsername[sourceAdminUsername]
	if !ok || targetID < 0 {
		return nil, Error{Message: "Admin " + sourceAdminUsername + " was not found in this backup"}
	}
	rows, err := db.users(ctx, adminByUsername)
	if err != nil {
		return nil, err
	}
	users := make([]ForeignUser, 0, len(rows))
	for _, row := range rows {
		if row.AdminID == targetID {
			users = append(users, row.User)
		}
	}
	return users, nil
}

// openForeignDatabase extracts the archive to a scratch directory and returns
// a read-only view matching the payload type recorded in its manifest. A file
// that isn't wrapped in Next's own tar/manifest format at all — a bare
// SQLite database file, or a plain MySQL/MariaDB dump — is also accepted
// directly, since Next descends from Marzban (and shares its core
// admins/users columns with Marzban forks like PasarGuard); this lets an
// admin migrate straight from those panels' own database exports without
// them ever going through Next's export format.
func openForeignDatabase(archivePath string) (foreignDatabase, error) {
	if stat, err := os.Stat(archivePath); err != nil || stat.IsDir() {
		return nil, Error{Message: "Backup file not found"}
	}
	if bare, ok, err := detectBareForeignDatabase(archivePath); ok || err != nil {
		return bare, err
	}
	extractDir, err := os.MkdirTemp("", "next-foreign-backup-*")
	if err != nil {
		return nil, err
	}
	cleanupExtractDir := func() { _ = os.RemoveAll(extractDir) }
	if err := safeExtract(archivePath, extractDir); err != nil {
		cleanupExtractDir()
		return nil, err
	}
	m, err := loadManifest(filepath.Join(extractDir, ManifestName))
	if err != nil {
		cleanupExtractDir()
		return nil, err
	}

	if m.Database.Payload != "" {
		payloadPath := filepath.Join(extractDir, filepath.FromSlash(m.Database.Payload))
		if !isRegularFile(payloadPath) {
			cleanupExtractDir()
			return nil, Error{Message: "Backup database payload is missing"}
		}
		switch m.Database.PayloadType {
		case "sqlite-file":
			db, err := openForeignSQLite(payloadPath)
			if err != nil {
				cleanupExtractDir()
				return nil, err
			}
			return &cleanupWrappedDB{foreignDatabase: db, cleanup: cleanupExtractDir}, nil
		case "mysql-dump":
			db, err := parseForeignMySQLDump(payloadPath)
			cleanupExtractDir()
			if err != nil {
				return nil, err
			}
			return db, nil
		default:
			cleanupExtractDir()
			return nil, Error{Message: "This backup's database payload type is not supported for user migration"}
		}
	}

	legacyPath := filepath.Join(extractDir, DatabaseDumpName)
	if !isRegularFile(legacyPath) {
		cleanupExtractDir()
		return nil, Error{Message: "Backup database payload is missing"}
	}
	db, err := parseForeignLegacyJSON(legacyPath)
	cleanupExtractDir()
	if err != nil {
		return nil, err
	}
	return db, nil
}

var sqliteFileMagic = []byte("SQLite format 3\x00")

// detectBareForeignDatabase sniffs the first bytes of the uploaded file to
// tell a Next-format archive (gzip) apart from a bare SQLite file or a plain
// text SQL dump uploaded directly from another panel. ok is false (with a nil
// error) when the file looks like a gzip stream and should go through the
// normal tar/manifest path instead.
func detectBareForeignDatabase(path string) (foreignDatabase, bool, error) {
	header := make([]byte, 16)
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	n, readErr := io.ReadFull(f, header)
	_ = f.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF {
		return nil, false, nil
	}
	header = header[:n]

	if len(header) >= 2 && header[0] == 0x1f && header[1] == 0x8b {
		return nil, false, nil // gzip: Next's own wrapped .rbbackup format
	}
	if bytes.HasPrefix(header, sqliteFileMagic) {
		db, err := openForeignSQLite(path)
		return db, true, err
	}
	db, err := parseForeignMySQLDump(path)
	return db, true, err
}

type cleanupWrappedDB struct {
	foreignDatabase
	cleanup func()
}

func (c *cleanupWrappedDB) close() {
	c.foreignDatabase.close()
	c.cleanup()
}

// --- SQLite -----------------------------------------------------------------

type foreignSQLiteDB struct {
	db *sql.DB
}

func openForeignSQLite(path string) (*foreignSQLiteDB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(abs) + "?mode=ro&immutable=1"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, Error{Message: "Could not open the backup's database file"}
	}
	return &foreignSQLiteDB{db: db}, nil
}

func (f *foreignSQLiteDB) close() { _ = f.db.Close() }

func (f *foreignSQLiteDB) admins(ctx context.Context) (map[int64]string, error) {
	columns, err := sqliteColumns(ctx, f.db, "admins")
	if err != nil || !containsAll(columns, "id", "username") {
		return nil, Error{Message: "This backup does not contain an admins table"}
	}
	rows, err := f.db.QueryContext(ctx, "SELECT id, username FROM admins")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[int64]string{}
	for rows.Next() {
		var id int64
		var username string
		if err := rows.Scan(&id, &username); err != nil {
			return nil, err
		}
		result[id] = username
	}
	return result, rows.Err()
}

var foreignUserColumns = []string{
	"username", "status", "data_limit", "expire", "note", "telegram_id",
	"contact_number", "on_hold_expire_duration", "on_hold_timeout", "ip_limit",
	"auto_delete_in_days", "data_limit_reset_strategy", "admin_id",
}

func (f *foreignSQLiteDB) users(ctx context.Context, _ map[string]int64) ([]foreignUserRow, error) {
	available, err := sqliteColumns(ctx, f.db, "users")
	if err != nil || !containsAll(available, "username", "admin_id") {
		return nil, Error{Message: "This backup does not contain a users table"}
	}
	selected := intersect(foreignUserColumns, available)
	query := "SELECT " + strings.Join(quoteSQLiteIdentifiers(selected), ", ") + " FROM users"
	rows, err := f.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanForeignUserRows(rows, selected)
}

func scanForeignUserRows(rows *sql.Rows, columns []string) ([]foreignUserRow, error) {
	result := []foreignUserRow{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		byName := map[string]any{}
		for i, col := range columns {
			byName[col] = values[i]
		}
		result = append(result, buildForeignUserRow(byName))
	}
	return result, rows.Err()
}

func buildForeignUserRow(v map[string]any) foreignUserRow {
	return foreignUserRow{
		AdminID: asInt64(v["admin_id"]),
		User: ForeignUser{
			Username:               asString(v["username"]),
			Status:                 asString(v["status"]),
			DataLimit:              asOptionalInt64(v["data_limit"]),
			Expire:                 asOptionalUnixTime(v["expire"]),
			Note:                   asString(v["note"]),
			TelegramID:             asOptionalInt64(v["telegram_id"]),
			ContactNumber:          asString(v["contact_number"]),
			OnHoldExpireDuration:   asOptionalInt64(v["on_hold_expire_duration"]),
			OnHoldTimeout:          asOptionalUnixTime(v["on_hold_timeout"]),
			IPLimit:                asOptionalInt64(v["ip_limit"]),
			AutoDeleteInDays:       asOptionalInt64(v["auto_delete_in_days"]),
			DataLimitResetStrategy: asString(v["data_limit_reset_strategy"]),
		},
	}
}

// --- Legacy JSON --------------------------------------------------------

type foreignJSONDB struct {
	tables map[string]legacyJSONTable
}

type legacyJSONTable struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

func parseForeignLegacyJSON(path string) (*foreignJSONDB, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Format string `json:"format"`
		Tables []struct {
			Name    string           `json:"name"`
			Columns []string         `json:"columns"`
			Rows    []map[string]any `json:"rows"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, Error{Message: "This backup's database payload could not be parsed"}
	}
	if payload.Format != Format {
		return nil, Error{Message: "Invalid database payload format"}
	}
	tables := map[string]legacyJSONTable{}
	for _, t := range payload.Tables {
		tables[t.Name] = legacyJSONTable{Columns: t.Columns, Rows: t.Rows}
	}
	return &foreignJSONDB{tables: tables}, nil
}

func (f *foreignJSONDB) close() {}

func (f *foreignJSONDB) admins(context.Context) (map[int64]string, error) {
	table, ok := f.tables["admins"]
	if !ok {
		return nil, Error{Message: "This backup does not contain an admins table"}
	}
	result := map[int64]string{}
	for _, row := range table.Rows {
		result[asInt64(row["id"])] = asString(row["username"])
	}
	return result, nil
}

func (f *foreignJSONDB) users(context.Context, map[string]int64) ([]foreignUserRow, error) {
	table, ok := f.tables["users"]
	if !ok {
		return nil, Error{Message: "This backup does not contain a users table"}
	}
	result := make([]foreignUserRow, 0, len(table.Rows))
	for _, row := range table.Rows {
		result = append(result, buildForeignUserRow(row))
	}
	return result, nil
}

// --- MySQL dump ----------------------------------------------------------

type foreignMySQLDB struct {
	adminsByID map[int64]string
	userRows   []foreignUserRow
}

func parseForeignMySQLDump(path string) (*foreignMySQLDB, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(content)

	adminCols, adminRows, err := extractMySQLInsertRows(text, "admins")
	if err != nil {
		return nil, err
	}
	if adminCols == nil {
		return nil, Error{Message: "This backup does not contain an admins table"}
	}
	admins := map[int64]string{}
	for _, row := range adminRows {
		byName := zip(adminCols, row)
		admins[asInt64(byName["id"])] = asString(byName["username"])
	}

	userCols, userRows, err := extractMySQLInsertRows(text, "users")
	if err != nil {
		return nil, err
	}
	if userCols == nil {
		return nil, Error{Message: "This backup does not contain a users table"}
	}
	users := make([]foreignUserRow, 0, len(userRows))
	for _, row := range userRows {
		users = append(users, buildForeignUserRow(zip(userCols, row)))
	}
	return &foreignMySQLDB{adminsByID: admins, userRows: users}, nil
}

func (f *foreignMySQLDB) close() {}

func (f *foreignMySQLDB) admins(context.Context) (map[int64]string, error) {
	return f.adminsByID, nil
}

func (f *foreignMySQLDB) users(context.Context, map[string]int64) ([]foreignUserRow, error) {
	return f.userRows, nil
}

// extractMySQLInsertRows scans a mysqldump text file for every
// "INSERT INTO `table` (cols) VALUES (...), (...), ...;" statement addressing
// the given table and returns the declared column list plus every value
// tuple, without ever executing any SQL from the dump.
func extractMySQLInsertRows(text string, table string) ([]string, [][]any, error) {
	pattern := regexp.MustCompile(fmt.Sprintf(`(?is)INSERT\s+INTO\s+`+"`?%s`?"+`\s*\(([^)]*)\)\s*VALUES\s*`, regexp.QuoteMeta(table)))
	var columns []string
	var rows [][]any
	searchFrom := 0
	for {
		loc := pattern.FindStringSubmatchIndex(text[searchFrom:])
		if loc == nil {
			break
		}
		colStart, colEnd := searchFrom+loc[2], searchFrom+loc[3]
		valuesStart := searchFrom + loc[1]
		cols := splitMySQLIdentifierList(text[colStart:colEnd])
		if columns == nil {
			columns = cols
		}
		tuples, consumed, err := splitMySQLValueTuples(text[valuesStart:])
		if err != nil {
			return nil, nil, err
		}
		for _, tuple := range tuples {
			values, err := parseMySQLValueTuple(tuple)
			if err != nil {
				return nil, nil, err
			}
			if len(values) != len(cols) {
				return nil, nil, Error{Message: "This backup's " + table + " table could not be parsed"}
			}
			rows = append(rows, values)
		}
		searchFrom = valuesStart + consumed
	}
	return columns, rows, nil
}

func splitMySQLIdentifierList(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "`")
		result = append(result, part)
	}
	return result
}

// splitMySQLValueTuples walks "(v1,v2,...),(v1,v2,...);" starting at the
// beginning of s and returns each parenthesized tuple's raw inner text plus
// how many bytes of s were consumed (through the terminating ';').
func splitMySQLValueTuples(s string) ([]string, int, error) {
	var tuples []string
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			return nil, i, Error{Message: "Unterminated INSERT statement in backup"}
		}
		if s[i] == ';' {
			return tuples, i + 1, nil
		}
		if s[i] != '(' {
			return nil, i, Error{Message: "Could not parse backup's INSERT statement"}
		}
		start := i + 1
		depth := 1
		i++
		inString := byte(0)
		for i < len(s) && depth > 0 {
			c := s[i]
			switch {
			case inString != 0:
				if c == '\\' {
					i++ // skip escaped char
				} else if c == inString {
					inString = 0
				}
			case c == '\'' || c == '"':
				inString = c
			case c == '(':
				depth++
			case c == ')':
				depth--
			}
			i++
		}
		if depth != 0 {
			return nil, i, Error{Message: "Unterminated value tuple in backup"}
		}
		tuples = append(tuples, s[start:i-1])
	}
	return nil, i, Error{Message: "Unterminated INSERT statement in backup"}
}

// parseMySQLValueTuple parses one comma-separated tuple of SQL literals
// (NULL, integers, or single/double-quoted strings with backslash and
// doubled-quote escaping) into Go values. It never evaluates the text as SQL.
func parseMySQLValueTuple(s string) ([]any, error) {
	var values []any
	i := 0
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\r' || s[i] == '\t') {
			i++
		}
		if i >= len(s) {
			break
		}
		switch {
		case s[i] == '\'' || s[i] == '"':
			quote := s[i]
			i++
			var b strings.Builder
			for i < len(s) {
				c := s[i]
				if c == '\\' && i+1 < len(s) {
					b.WriteByte(unescapeMySQLChar(s[i+1]))
					i += 2
					continue
				}
				if c == quote {
					if i+1 < len(s) && s[i+1] == quote {
						b.WriteByte(quote)
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteByte(c)
				i++
			}
			values = append(values, b.String())
		default:
			start := i
			for i < len(s) && s[i] != ',' {
				i++
			}
			token := strings.TrimSpace(s[start:i])
			if strings.EqualFold(token, "NULL") {
				values = append(values, nil)
			} else {
				values = append(values, token)
			}
		}
		for i < len(s) && s[i] != ',' {
			i++
		}
		if i < len(s) && s[i] == ',' {
			i++
		}
	}
	return values, nil
}

func unescapeMySQLChar(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	case '0':
		return 0
	default:
		return c
	}
}

func zip(columns []string, values []any) map[string]any {
	result := make(map[string]any, len(columns))
	for i, col := range columns {
		if i < len(values) {
			result[col] = values[i]
		}
	}
	return result
}

// --- shared helpers --------------------------------------------------------

func containsAll(haystack []string, needles ...string) bool {
	set := map[string]bool{}
	for _, v := range haystack {
		set[v] = true
	}
	for _, needle := range needles {
		if !set[needle] {
			return false
		}
	}
	return true
}

func intersect(preferred []string, available []string) []string {
	set := map[string]bool{}
	for _, v := range available {
		set[v] = true
	}
	result := make([]string, 0, len(preferred))
	for _, v := range preferred {
		if set[v] {
			result = append(result, v)
		}
	}
	return result
}

func quoteSQLiteIdentifiers(names []string) []string {
	result := make([]string, len(names))
	for i, name := range names {
		result[i] = quoteSQLiteIdentifier(name)
	}
	return result
}

func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func asInt64(v any) int64 {
	n, _ := asOptionalInt64Err(v)
	return n
}

func asOptionalInt64(v any) *int64 {
	n, ok := asOptionalInt64Err(v)
	if !ok {
		return nil
	}
	return &n
}

func asOptionalInt64Err(v any) (int64, bool) {
	switch t := v.(type) {
	case nil:
		return 0, false
	case int64:
		return t, true
	case int:
		return int64(t), true
	case float64:
		return int64(t), true
	case []byte:
		n, err := strconv.ParseInt(strings.TrimSpace(string(t)), 10, 64)
		return n, err == nil
	case string:
		trimmed := strings.TrimSpace(t)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		return n, err == nil
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	default:
		return 0, false
	}
}

// asOptionalUnixTime reads a timestamp field that may be stored either as a
// Unix integer (Marzban's and Next's own "expire"/"on_hold_timeout" columns)
// or as an actual DATETIME value (PasarGuard's), returning it as a Unix
// timestamp either way.
func asOptionalUnixTime(v any) *int64 {
	if n, ok := asOptionalInt64Err(v); ok {
		return &n
	}
	var text string
	switch t := v.(type) {
	case time.Time:
		unix := t.Unix()
		return &unix
	case []byte:
		text = string(t)
	case string:
		text = t
	default:
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if parsed, err := time.Parse(layout, text); err == nil {
			unix := parsed.Unix()
			return &unix
		}
	}
	return nil
}
