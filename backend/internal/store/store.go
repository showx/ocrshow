package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL,
    skip_vl INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    log TEXT NOT NULL DEFAULT '',
    image_count INTEGER NOT NULL DEFAULT 0,
    record_count INTEGER NOT NULL DEFAULT 0,
    detected_types TEXT NOT NULL DEFAULT '[]',
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    started_at TEXT NOT NULL DEFAULT '',
    finished_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS job_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    filename TEXT NOT NULL,
    stored_name TEXT NOT NULL,
    date TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id TEXT NOT NULL,
    sheet_type TEXT NOT NULL,
    image TEXT NOT NULL DEFAULT '',
    date TEXT NOT NULL DEFAULT '',
    rank INTEGER NOT NULL DEFAULT 0,
    excel_row INTEGER NOT NULL DEFAULT 0,
    col_group INTEGER NOT NULL DEFAULT 0,
    app_name TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_files_job ON job_files(job_id);
CREATE INDEX IF NOT EXISTS idx_records_job ON records(job_id);
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
`)
	return err
}

func (s *Store) CreateJob(job *Job, files []JobFile) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(
		`INSERT INTO jobs (id, category, skip_vl, status, image_count, created_at)
         VALUES (?, ?, ?, ?, ?, ?)`,
		job.ID, job.Category, boolToInt(job.SkipVL), job.Status, job.ImageCount, job.CreatedAt,
	)
	if err != nil {
		return err
	}
	for _, f := range files {
		if _, err := tx.Exec(
			`INSERT INTO job_files (job_id, filename, stored_name, date) VALUES (?, ?, ?, ?)`,
			job.ID, f.Filename, f.StoredName, f.Date,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListJobs() ([]Job, error) {
	rows, err := s.db.Query(`
SELECT id, category, skip_vl, status, error, image_count, record_count, detected_types, created_at, started_at, finished_at
FROM jobs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJobList(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	if out == nil {
		out = []Job{}
	}
	return out, rows.Err()
}

func (s *Store) GetJob(id string, withRecords bool) (*Job, error) {
	row := s.db.QueryRow(`
SELECT id, category, skip_vl, status, error, log, image_count, record_count, detected_types, summary, created_at, started_at, finished_at
FROM jobs WHERE id = ?`, id)
	j, err := scanJobDetail(row)
	if err != nil {
		return nil, err
	}
	files, err := s.listFiles(id)
	if err != nil {
		return nil, err
	}
	j.Files = files
	if withRecords {
		recs, err := s.ListRecords(id)
		if err != nil {
			return nil, err
		}
		j.Records = recs
	}
	return &j, nil
}

func (s *Store) listFiles(jobID string) ([]JobFile, error) {
	rows, err := s.db.Query(`SELECT id, job_id, filename, stored_name, date FROM job_files WHERE job_id = ? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []JobFile
	for rows.Next() {
		var f JobFile
		if err := rows.Scan(&f.ID, &f.JobID, &f.Filename, &f.StoredName, &f.Date); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if out == nil {
		out = []JobFile{}
	}
	return out, rows.Err()
}

func (s *Store) ListRecords(jobID string) ([]Record, error) {
	rows, err := s.db.Query(`
SELECT id, job_id, sheet_type, image, date, rank, excel_row, col_group, app_name, payload
FROM records WHERE job_id = ? ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var payload string
		if err := rows.Scan(&r.ID, &r.JobID, &r.SheetType, &r.Image, &r.Date, &r.Rank, &r.ExcelRow, &r.ColGroup, &r.AppName, &payload); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(payload), &r.Payload)
		if r.Payload == nil {
			r.Payload = map[string]any{}
		}
		out = append(out, r)
	}
	if out == nil {
		out = []Record{}
	}
	return out, rows.Err()
}

func (s *Store) UpdateStatus(id, status, errMsg string) error {
	now := NowISO()
	switch status {
	case "running":
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, error = ?, started_at = ? WHERE id = ?`, status, errMsg, now, id)
		return err
	case "succeeded", "failed":
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, error = ?, finished_at = ? WHERE id = ?`, status, errMsg, now, id)
		return err
	default:
		_, err := s.db.Exec(`UPDATE jobs SET status = ?, error = ? WHERE id = ?`, status, errMsg, id)
		return err
	}
}

func (s *Store) AppendLog(id, chunk string) error {
	if strings.TrimSpace(chunk) == "" {
		return nil
	}
	_, err := s.db.Exec(`
UPDATE jobs SET log = CASE
    WHEN length(log) + length(?) > 12000 THEN substr(log || ?, -12000)
    ELSE log || ?
END WHERE id = ?`, chunk, chunk, chunk, id)
	return err
}

func (s *Store) SaveResult(id string, records []map[string]any, summary any, detected []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM records WHERE job_id = ?`, id); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
INSERT INTO records (job_id, sheet_type, image, date, rank, excel_row, col_group, app_name, payload)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, rec := range records {
		payload, _ := json.Marshal(rec)
		_, err := stmt.Exec(
			id,
			AsString(rec["sheet_type"]),
			AsString(rec["image"]),
			AsString(rec["date"]),
			asInt(rec["rank"]),
			asInt(rec["excel_row"]),
			asInt(rec["col_group"]),
			AsString(rec["app_name"]),
			string(payload),
		)
		if err != nil {
			return err
		}
	}
	sumJSON, _ := json.Marshal(summary)
	typesJSON, _ := json.Marshal(detected)
	_, err = tx.Exec(
		`UPDATE jobs SET record_count = ?, summary = ?, detected_types = ?, status = ?, error = '', finished_at = ? WHERE id = ?`,
		len(records), string(sumJSON), string(typesJSON), "succeeded", NowISO(), id,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReplaceRecords(id string, records []map[string]any) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM records WHERE job_id = ?`, id); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
INSERT INTO records (job_id, sheet_type, image, date, rank, excel_row, col_group, app_name, payload)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, rec := range records {
		if rec == nil {
			rec = map[string]any{}
		}
		payload, _ := json.Marshal(rec)
		if _, err := stmt.Exec(
			id,
			AsString(rec["sheet_type"]),
			AsString(rec["image"]),
			AsString(rec["date"]),
			asInt(rec["rank"]),
			asInt(rec["excel_row"]),
			asInt(rec["col_group"]),
			AsString(rec["app_name"]),
			string(payload),
		); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE jobs SET record_count = ? WHERE id = ?`, len(records), id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListPending() ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM jobs WHERE status IN ('pending', 'running') ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) DeleteJob(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM records WHERE job_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM job_files WHERE job_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM jobs WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func scanJobList(rows *sql.Rows) (Job, error) {
	var j Job
	var typesJSON string
	var skipVL int
	if err := rows.Scan(&j.ID, &j.Category, &skipVL, &j.Status, &j.Error, &j.ImageCount, &j.RecordCount, &typesJSON, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
		return j, err
	}
	j.SkipVL = skipVL != 0
	_ = json.Unmarshal([]byte(typesJSON), &j.DetectedTypes)
	return j, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJobDetail(row scanner) (Job, error) {
	var j Job
	var skipVL int
	var typesJSON, summaryJSON string
	if err := row.Scan(&j.ID, &j.Category, &skipVL, &j.Status, &j.Error, &j.Log, &j.ImageCount, &j.RecordCount, &typesJSON, &summaryJSON, &j.CreatedAt, &j.StartedAt, &j.FinishedAt); err != nil {
		return j, err
	}
	j.SkipVL = skipVL != 0
	_ = json.Unmarshal([]byte(typesJSON), &j.DetectedTypes)
	if strings.TrimSpace(summaryJSON) != "" {
		_ = json.Unmarshal([]byte(summaryJSON), &j.Summary)
	}
	return j, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func asInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}
