package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Root       string
	DataDir    string
	UploadDir  string
	ResultDir  string
	Python     string
	Pipeline   string
	Addr       string
	Frontend   string
	Device     string
	VLModel    string
	VLHost     string
	TimeoutSec int
	Categories []Category
}

type Category struct {
	ID       string   `toml:"id" json:"id"`
	Name     string   `toml:"name" json:"name"`
	Desc     string   `toml:"desc" json:"desc"`
	Priority int      `toml:"priority" json:"-"`
	Columns  []Column `toml:"columns" json:"columns,omitempty"`
}

type Column struct {
	Key   string `toml:"key" json:"key"`
	Label string `toml:"label" json:"label"`
}

type fileConfig struct {
	Device     string     `toml:"device"`
	Addr       string     `toml:"addr"`
	Paths      filePaths  `toml:"paths"`
	VL         fileVL     `toml:"vl"`
	Categories []Category `toml:"categories"`
}

type filePaths struct {
	Images string `toml:"images"`
	Output string `toml:"output"`
	Data   string `toml:"data"`
}

type fileVL struct {
	Skip    bool   `toml:"skip"`
	Host    string `toml:"host"`
	Model   string `toml:"model"`
	Timeout int    `toml:"timeout"`
	APIKey  string `toml:"api_key"`
}

func DefaultCategories() []Category {
	return []Category{autoCategory()}
}

func autoCategory() Category {
	return Category{
		ID:   "auto",
		Name: "自动识别",
		Desc: "按表头匹配已安装的版式模块；没有模块时走通用表格",
		Columns: []Column{
			{Key: "rank", Label: "序号"},
			{Key: "app_name", Label: "名称"},
			{Key: "image", Label: "图片"},
		},
	}
}

func scanSheetCatalog(root string) []Category {
	dir := filepath.Join(root, "sheets")
	var out []Category
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".toml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var cat Category
		if err := toml.Unmarshal(data, &cat); err != nil || strings.TrimSpace(cat.ID) == "" {
			return nil
		}
		out = append(out, cat)
		return nil
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (c Config) CategoryList() []Category {
	scanned := scanSheetCatalog(c.Root)
	byID := map[string]Category{}
	for _, item := range c.Categories {
		if item.ID != "" {
			byID[item.ID] = item
		}
	}
	out := []Category{autoCategory()}
	seen := map[string]bool{"auto": true}
	for _, d := range scanned {
		if o, ok := byID[d.ID]; ok {
			if strings.TrimSpace(o.Name) != "" {
				d.Name = o.Name
			}
			if strings.TrimSpace(o.Desc) != "" {
				d.Desc = o.Desc
			}
			if len(o.Columns) > 0 {
				d.Columns = o.Columns
			}
		}
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		out = append(out, d)
	}
	return out
}

func Load() Config {
	root := envOr("OCR_ROOT", findRoot())
	loadEnvFile(filepath.Join(root, ".env"))

	device := "auto"
	addr := ":8080"
	dataDir := "data"
	vlHost := "http://127.0.0.1:11434"
	vlModel := "qwen3-vl:8b"
	timeout := 1800
	var cats []Category

	for _, name := range []string{"config.example.toml", "config.toml"} {
		fc := loadTomlFile(filepath.Join(root, name))
		applyFileConfig(fc, &device, &addr, &dataDir, &vlHost, &vlModel, &timeout)
		if len(fc.Categories) > 0 {
			cats = fc.Categories
		}
	}

	device = envOr("OCR_DEVICE", device)
	addr = envOr("OCR_ADDR", addr)
	dataDir = envOr("OCR_DATA", dataDir)
	vlHost = envOr("OCR_VL_HOST", vlHost)
	vlModel = envOr("OCR_VL_MODEL", vlModel)
	if v := strings.TrimSpace(os.Getenv("OCR_VL_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}

	dataAbs := absFromRoot(root, dataDir, "data")
	return Config{
		Root:       root,
		DataDir:    dataAbs,
		UploadDir:  filepath.Join(dataAbs, "uploads"),
		ResultDir:  filepath.Join(dataAbs, "results"),
		Python:     envOr("OCR_PYTHON", findPython(root)),
		Pipeline:   filepath.Join(root, "pipeline.py"),
		Addr:       addr,
		Frontend:   envOr("OCR_FRONTEND", filepath.Join(root, "frontend", "dist")),
		Device:     device,
		VLModel:    vlModel,
		VLHost:     vlHost,
		TimeoutSec: timeout,
		Categories: cats,
	}
}

func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "export ") {
			line = strings.TrimSpace(line[7:])
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if q := val[0]; (q == '"' || q == '\'') && val[len(val)-1] == q {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

func loadTomlFile(path string) fileConfig {
	var fc fileConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return fc
	}
	_ = toml.Unmarshal(data, &fc)
	return fc
}

func applyFileConfig(fc fileConfig, device, addr, dataDir, vlHost, vlModel *string, timeout *int) {
	if v := strings.TrimSpace(fc.Device); v != "" {
		*device = v
	}
	if v := strings.TrimSpace(fc.Addr); v != "" {
		*addr = v
	}
	if v := strings.TrimSpace(fc.Paths.Data); v != "" {
		*dataDir = v
	}
	if v := strings.TrimSpace(fc.VL.Host); v != "" {
		*vlHost = v
	}
	if v := strings.TrimSpace(fc.VL.Model); v != "" {
		*vlModel = v
	}
	if fc.VL.Timeout > 0 {
		*timeout = fc.VL.Timeout
	}
	if v := strings.TrimSpace(fc.VL.APIKey); v != "" {
		if _, exists := os.LookupEnv("OCR_VL_API_KEY"); !exists {
			_ = os.Setenv("OCR_VL_API_KEY", v)
		}
	}
}

func absFromRoot(root, p, fallback string) string {
	if strings.TrimSpace(p) == "" {
		p = fallback
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func findRoot() string {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		starts = append(starts, filepath.Dir(file))
	}
	for _, start := range starts {
		dir := start
		for i := 0; i < 8; i++ {
			if _, err := os.Stat(filepath.Join(dir, "pipeline.py")); err == nil {
				return dir
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	cwd, _ := os.Getwd()
	return cwd
}

func findPython(root string) string {
	candidates := []string{
		filepath.Join(root, ".venv", "Scripts", "python.exe"),
		filepath.Join(root, ".venv", "bin", "python"),
		filepath.Join(root, ".venv", "bin", "python3"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if runtime.GOOS == "windows" {
		return "python"
	}
	return "python3"
}
