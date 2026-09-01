package router

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ocrshow/internal/config"
	"ocrshow/internal/handler"
	"ocrshow/internal/middleware"
	"ocrshow/internal/store"
	"ocrshow/internal/worker"
)

func New(cfg config.Config, st *store.Store, w *worker.Worker) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 64 << 20
	r.Use(gin.Logger(), gin.Recovery(), middleware.CORS())

	api := handler.New(cfg, st, w)
	g := r.Group("/api")
	{
		g.GET("/health", api.Health)
		g.POST("/login", api.Login)
		g.POST("/logout", api.Logout)
		g.GET("/me", middleware.OptionalAuth(st), api.Me)

		auth := g.Group("")
		auth.Use(middleware.RequireAuth(st))
		{
			auth.GET("/categories", api.Categories)
			auth.GET("/jobs", api.ListJobs)
			auth.POST("/jobs", api.CreateJob)
			auth.GET("/jobs/:id", api.GetJob)
			auth.PUT("/jobs/:id/records", api.UpdateRecords)
			auth.DELETE("/jobs/:id", api.DeleteJob)
			auth.GET("/jobs/:id/files/:name", api.ServeFile)
			auth.GET("/jobs/:id/export", api.ExportJob)
		}
	}

	mountFrontend(r, cfg.Frontend)
	return r
}

func mountFrontend(r *gin.Engine, dist string) {
	if dist == "" {
		return
	}
	st, err := os.Stat(dist)
	if err != nil || !st.IsDir() {
		return
	}
	r.Static("/assets", filepath.Join(dist, "assets"))
	index := filepath.Join(dist, "index.html")
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		c.File(index)
	})
}
