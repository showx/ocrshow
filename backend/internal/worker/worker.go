package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ocrshow/internal/config"
	"ocrshow/internal/store"
)

type Worker struct {
	cfg   config.Config
	store *store.Store
	queue chan string
	once  sync.Once
}

func New(cfg config.Config, st *store.Store) *Worker {
	return &Worker{
		cfg:   cfg,
		store: st,
		queue: make(chan string, 64),
	}
}

func (w *Worker) Start() {
	w.once.Do(func() {
		go w.loop()
	})
}

func (w *Worker) Enqueue(id string) {
	select {
	case w.queue <- id:
	default:
		go func() { w.queue <- id }()
	}
}

func (w *Worker) RecoverPending() error {
	ids, err := w.store.ListPending()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := w.store.UpdateStatus(id, "pending", ""); err != nil {
			log.Printf("重置任务 %s 失败: %v", id, err)
			continue
		}
		w.Enqueue(id)
	}
	if len(ids) > 0 {
		log.Printf("已重新排队 %d 个未完成任务", len(ids))
	}
	return nil
}

func (w *Worker) loop() {
	for id := range w.queue {
		if err := w.runJob(id); err != nil {
			log.Printf("任务 %s 失败: %v", id, err)
			_ = w.store.UpdateStatus(id, "failed", err.Error())
			_ = w.store.AppendLog(id, "\n[失败] "+err.Error()+"\n")
		}
	}
}

func (w *Worker) runJob(id string) error {
	job, err := w.store.GetJob(id, false)
	if err != nil {
		return fmt.Errorf("读取任务: %w", err)
	}
	if err := w.store.UpdateStatus(id, "running", ""); err != nil {
		return err
	}

	resultDir := filepath.Join(w.cfg.ResultDir, id)
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return err
	}

	args := []string{w.cfg.Pipeline, "--out", resultDir, "--device", w.cfg.Device, "--sheet-type", job.Category}
	if job.SkipVL {
		args = append(args, "--skip-vl")
	} else {
		args = append(args, "--vl-model", w.cfg.VLModel, "--vl-host", w.cfg.VLHost)
	}

	uploadDir := filepath.Join(w.cfg.UploadDir, id)
	for _, f := range job.Files {
		args = append(args, filepath.Join(uploadDir, f.StoredName))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(w.cfg.TimeoutSec)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.cfg.Python, args...)
	cmd.Dir = w.cfg.Root
	cmd.Env = append(os.Environ(), "PYTHONUNBUFFERED=1", "PYTHONIOENCODING=utf-8")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动识别进程失败（python=%s）: %w", w.cfg.Python, err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); w.pipeLog(id, stdout) }()
	go func() { defer wg.Done(); w.pipeLog(id, stderr) }()

	waitErr := cmd.Wait()
	wg.Wait()
	if waitErr != nil {
		return fmt.Errorf("识别进程退出: %w", waitErr)
	}

	resultPath := filepath.Join(resultDir, "result.json")
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return fmt.Errorf("读取结果 %s: %w", resultPath, err)
	}
	var payload struct {
		OK      bool             `json:"ok"`
		Records []map[string]any `json:"records"`
		Summary any              `json:"summary"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("解析 result.json: %w", err)
	}
	return w.store.SaveResult(id, payload.Records, payload.Summary, uniqueSheetTypes(payload.Records))
}

func (w *Worker) pipeLog(id string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var buf strings.Builder
	lastFlush := time.Now()
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		_ = w.store.AppendLog(id, buf.String())
		buf.Reset()
		lastFlush = time.Now()
	}
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("[%s] %s", id, line)
		buf.WriteString(line)
		buf.WriteByte('\n')
		if buf.Len() > 800 || time.Since(lastFlush) > 1500*time.Millisecond {
			flush()
		}
	}
	flush()
}

func uniqueSheetTypes(records []map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, rec := range records {
		t := store.AsString(rec["sheet_type"])
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}
