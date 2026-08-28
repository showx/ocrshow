package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ocrshow/internal/config"
	"ocrshow/internal/store"
	"ocrshow/internal/worker"
)

var allowedExt = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".bmp": true, ".tif": true, ".tiff": true,
}

type API struct {
	cfg    config.Config
	store  *store.Store
	worker *worker.Worker
}

func (a *API) allowedCategory(id string) bool {
	for _, cat := range a.cfg.CategoryList() {
		if cat.ID == id {
			return true
		}
	}
	return false
}

func New(cfg config.Config, st *store.Store, w *worker.Worker) *API {
	return &API{cfg: cfg, store: st, worker: w}
}

func (a *API) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"ok":     true,
		"python": a.cfg.Python,
		"root":   a.cfg.Root,
	})
}

func (a *API) Categories(c *gin.Context) {
	c.JSON(http.StatusOK, a.cfg.CategoryList())
}

func (a *API) ListJobs(c *gin.Context) {
	jobs, err := a.store.ListJobs()
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, jobs)
}

func (a *API) GetJob(c *gin.Context) {
	job, err := a.store.GetJob(c.Param("id"), true)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "任务不存在")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, job)
}

func (a *API) CreateJob(c *gin.Context) {
	category := strings.TrimSpace(c.PostForm("category"))
	if category == "" {
		category = "auto"
	}
	if !a.allowedCategory(category) {
		fail(c, http.StatusBadRequest, "类别无效")
		return
	}
	skipVL := true
	switch strings.ToLower(strings.TrimSpace(c.PostForm("skip_vl"))) {
	case "0", "false", "no", "off":
		skipVL = false
	}

	form, err := c.MultipartForm()
	if err != nil {
		fail(c, http.StatusBadRequest, "请以 multipart 上传图片")
		return
	}
	headers := form.File["files"]
	if len(headers) == 0 {
		headers = form.File["file"]
	}
	if len(headers) == 0 {
		fail(c, http.StatusBadRequest, "请至少上传一张图片")
		return
	}
	if len(headers) > 12 {
		fail(c, http.StatusBadRequest, "一次最多 12 张图")
		return
	}

	id := newJobID()
	uploadDir := filepath.Join(a.cfg.UploadDir, id)
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}

	var files []store.JobFile
	used := map[string]int{}
	for _, fh := range headers {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if !allowedExt[ext] {
			fail(c, http.StatusBadRequest, "不支持的文件类型: "+fh.Filename)
			return
		}
		if fh.Size > 30<<20 {
			fail(c, http.StatusBadRequest, "单张图片不能超过 30MB")
			return
		}
		stored := uniqueName(sanitizeFilename(fh.Filename), used)
		dst := filepath.Join(uploadDir, stored)
		if err := c.SaveUploadedFile(fh, dst); err != nil {
			fail(c, http.StatusInternalServerError, "保存图片失败: "+err.Error())
			return
		}
		files = append(files, store.JobFile{
			Filename:   fh.Filename,
			StoredName: stored,
			Date:       dateFromName(stored),
		})
	}

	job := &store.Job{
		ID:         id,
		Category:   category,
		SkipVL:     skipVL,
		Status:     "pending",
		ImageCount: len(files),
		CreatedAt:  store.NowISO(),
	}
	if err := a.store.CreateJob(job, files); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	a.worker.Enqueue(id)
	created, _ := a.store.GetJob(id, false)
	c.JSON(http.StatusAccepted, created)
}

func (a *API) DeleteJob(c *gin.Context) {
	id := c.Param("id")
	if _, err := a.store.GetJob(id, false); errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "任务不存在")
		return
	} else if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	_ = os.RemoveAll(filepath.Join(a.cfg.UploadDir, id))
	_ = os.RemoveAll(filepath.Join(a.cfg.ResultDir, id))
	if err := a.store.DeleteJob(id); err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *API) ServeFile(c *gin.Context) {
	id := c.Param("id")
	name := filepath.Base(c.Param("name"))
	path := filepath.Join(a.cfg.UploadDir, id, name)
	if _, err := os.Stat(path); err != nil {
		fail(c, http.StatusNotFound, "文件不存在")
		return
	}
	c.File(path)
}

func (a *API) ExportJob(c *gin.Context) {
	job, err := a.store.GetJob(c.Param("id"), true)
	if errors.Is(err, sql.ErrNoRows) {
		fail(c, http.StatusNotFound, "任务不存在")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, err.Error())
		return
	}
	payloads := make([]map[string]any, 0, len(job.Records))
	for _, rec := range job.Records {
		payloads = append(payloads, rec.Payload)
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="ocr-%s.json"`, job.ID))
	c.JSON(http.StatusOK, payloads)
}

func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"error": msg})
}

func newJobID() string {
	b := make([]byte, 4)
	_, _ = io.ReadFull(rand.Reader, b)
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

var unsafeName = regexp.MustCompile(`[^\w.\p{Han}-]+`)

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = unsafeName.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == ".." {
		name = "image.jpg"
	}
	return name
}

func uniqueName(name string, used map[string]int) string {
	base := name
	if used[base] == 0 {
		used[base] = 1
		return base
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	used[base]++
	return fmt.Sprintf("%s_%d%s", stem, used[base], ext)
}

func dateFromName(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	reFull := regexp.MustCompile(`(?:^|[^0-9])((?:19|20)\d{2})(\d{2})(\d{2})(?:[^0-9]|$)`)
	if m := reFull.FindStringSubmatch(stem); len(m) == 4 {
		return m[1] + "-" + m[2] + "-" + m[3]
	}
	return ""
}
