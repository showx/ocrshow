package main

import (
	"log"
	"os"
	"path/filepath"

	"ocrshow/internal/config"
	"ocrshow/internal/router"
	"ocrshow/internal/store"
	"ocrshow/internal/worker"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		log.Fatalf("创建上传目录失败: %v", err)
	}

	st, err := store.Open(filepath.Join(cfg.DataDir, "ocrshow.db"))
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer st.Close()

	w := worker.New(cfg, st)
	w.Start()
	if err := w.RecoverPending(); err != nil {
		log.Printf("恢复排队任务失败: %v", err)
	}

	engine := router.New(cfg, st, w)
	log.Printf("OCR Show 服务启动 %s  （数据目录 %s）", cfg.Addr, cfg.DataDir)
	if err := engine.Run(cfg.Addr); err != nil {
		log.Fatal(err)
	}
}
