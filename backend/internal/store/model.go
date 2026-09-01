package store

import (
	"fmt"
	"time"
)

type Job struct {
	ID            string    `json:"id"`
	Category      string    `json:"category"`
	SkipVL        bool      `json:"skip_vl"`
	Status        string    `json:"status"`
	Error         string    `json:"error,omitempty"`
	Log           string    `json:"log,omitempty"`
	ImageCount    int       `json:"image_count"`
	RecordCount   int       `json:"record_count"`
	DetectedTypes []string  `json:"detected_types,omitempty"`
	Summary       any       `json:"summary,omitempty"`
	CreatedAt     string    `json:"created_at"`
	StartedAt     string    `json:"started_at,omitempty"`
	FinishedAt    string    `json:"finished_at,omitempty"`
	Files         []JobFile `json:"files,omitempty"`
	Records       []Record  `json:"records,omitempty"`
}

type JobFile struct {
	ID         int64  `json:"id"`
	JobID      string `json:"job_id"`
	Filename   string `json:"filename"`
	StoredName string `json:"stored_name"`
	Date       string `json:"date,omitempty"`
}

type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type AuthUserSeed struct {
	Username string
	Password string
}

type Record struct {
	ID        int64          `json:"id"`
	JobID     string         `json:"job_id"`
	SheetType string         `json:"sheet_type"`
	Image     string         `json:"image"`
	Date      string         `json:"date"`
	Rank      int            `json:"rank"`
	ExcelRow  int            `json:"excel_row"`
	ColGroup  int            `json:"col_group"`
	AppName   string         `json:"app_name"`
	Payload   map[string]any `json:"payload"`
}

func NowISO() string {
	return time.Now().Format("2006-01-02T15:04:05")
}

func AsString(v any) string {
	if v == nil {
		return ""
	}
	if t, ok := v.(string); ok {
		return t
	}
	return fmt.Sprint(v)
}
