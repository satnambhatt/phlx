// Package db owns the single SQLite file at ~/.phalanx/phalanx.db.
// Used directly by hooks, CLI, and scanner. No daemon, no server.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	DB      *sql.DB
	DataDir string
	DBPath  string
)

func Open() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	DataDir = filepath.Join(home, ".phalanx")
	if err := os.MkdirAll(DataDir, 0o755); err != nil {
		return err
	}
	DBPath = filepath.Join(DataDir, "phalanx.db")

	d, err := sql.Open("sqlite", DBPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return err
	}
	DB = d
	return migrate()
}

func migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS scans (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		pkg         TEXT NOT NULL,
		version     TEXT NOT NULL,
		ecosystem   TEXT NOT NULL DEFAULT 'npm',
		result      TEXT NOT NULL,
		policy      TEXT NOT NULL,
		scanned_at  INTEGER NOT NULL,
		UNIQUE(pkg, version, ecosystem)
	);
	CREATE INDEX IF NOT EXISTS idx_scans_pkg ON scans(pkg, ecosystem);

	CREATE TABLE IF NOT EXISTS installs (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		pkg          TEXT NOT NULL,
		version      TEXT NOT NULL,
		ecosystem    TEXT NOT NULL,
		action       TEXT NOT NULL,
		project      TEXT,
		installed_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_installs_time ON installs(installed_at);

	CREATE TABLE IF NOT EXISTS watched (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		path      TEXT NOT NULL UNIQUE,
		added_at  INTEGER NOT NULL,
		last_scan INTEGER
	);

	CREATE TABLE IF NOT EXISTS config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);`
	if _, err := DB.Exec(schema); err != nil {
		return err
	}

	defaults := map[string]string{
		"block_on_critical":  "true",
		"block_on_high_nfix": "true",
		"warn_on_medium":     "true",
		"block_on_gpl":       "true",
		"notify_terminal":    "true",
		"installed_version":  "1.0.0",
	}
	for k, v := range defaults {
		if _, err := DB.Exec(`INSERT OR IGNORE INTO config (key, value) VALUES (?, ?)`, k, v); err != nil {
			return err
		}
	}
	return nil
}

func GetConfig(key string) string {
	var v string
	if err := DB.QueryRow(`SELECT value FROM config WHERE key=?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

func SetConfig(key, value string) error {
	_, err := DB.Exec(`INSERT OR REPLACE INTO config (key, value) VALUES (?, ?)`, key, value)
	return err
}

type Install struct {
	Pkg       string
	Version   string
	Ecosystem string
	Action    string
	Project   string
	Time      time.Time
}

func RecordInstall(in Install) error {
	if in.Project == "" {
		if wd, err := os.Getwd(); err == nil {
			in.Project = wd
		}
	}
	_, err := DB.Exec(
		`INSERT INTO installs (pkg, version, ecosystem, action, project, installed_at) VALUES (?,?,?,?,?,?)`,
		in.Pkg, in.Version, in.Ecosystem, in.Action, in.Project, time.Now().UnixMilli(),
	)
	return err
}

func IsAllowed(pkg, version, ecosystem string) bool {
	if GetConfig(fmt.Sprintf("allow:%s:%s:%s", ecosystem, pkg, version)) != "" {
		return true
	}
	return GetConfig(fmt.Sprintf("allow:%s:%s:*", ecosystem, pkg)) != ""
}

type Stats struct {
	TotalScans int
	Blocked    int
	Warned     int
	Allowed    int
	Watched    int
}

func GetStats() (Stats, error) {
	var s Stats
	if err := DB.QueryRow(`SELECT COUNT(*) FROM scans`).Scan(&s.TotalScans); err != nil {
		return s, err
	}
	for _, row := range []struct {
		action string
		dst    *int
	}{
		{"blocked", &s.Blocked},
		{"warned", &s.Warned},
		{"allowed", &s.Allowed},
	} {
		if err := DB.QueryRow(`SELECT COUNT(*) FROM installs WHERE action=?`, row.action).Scan(row.dst); err != nil {
			return s, err
		}
	}
	if err := DB.QueryRow(`SELECT COUNT(*) FROM watched`).Scan(&s.Watched); err != nil {
		return s, err
	}
	return s, nil
}

type HistoryRow struct {
	Pkg, Version, Ecosystem, Action string
	InstalledAt                     time.Time
}

func History(limit int) ([]HistoryRow, error) {
	rows, err := DB.Query(`SELECT pkg, version, ecosystem, action, installed_at FROM installs ORDER BY installed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryRow
	for rows.Next() {
		var r HistoryRow
		var ms int64
		if err := rows.Scan(&r.Pkg, &r.Version, &r.Ecosystem, &r.Action, &ms); err != nil {
			return nil, err
		}
		r.InstalledAt = time.UnixMilli(ms)
		out = append(out, r)
	}
	return out, nil
}

func GetCachedScan(pkg, version, ecosystem string, ttl time.Duration) (string, string, bool) {
	var result, policy string
	cutoff := time.Now().Add(-ttl).UnixMilli()
	err := DB.QueryRow(
		`SELECT result, policy FROM scans WHERE pkg=? AND version=? AND ecosystem=? AND scanned_at > ?`,
		pkg, version, ecosystem, cutoff,
	).Scan(&result, &policy)
	if err != nil {
		return "", "", false
	}
	return result, policy, true
}

func StoreScan(pkg, version, ecosystem, result, policy string) error {
	_, err := DB.Exec(
		`INSERT OR REPLACE INTO scans (pkg,version,ecosystem,result,policy,scanned_at) VALUES (?,?,?,?,?,?)`,
		pkg, version, ecosystem, result, policy, time.Now().UnixMilli(),
	)
	return err
}

func ListConfig() (map[string]string, error) {
	rows, err := DB.Query(`SELECT key, value FROM config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func AddWatched(path string) error {
	_, err := DB.Exec(`INSERT OR IGNORE INTO watched (path, added_at) VALUES (?, ?)`, path, time.Now().UnixMilli())
	return err
}
