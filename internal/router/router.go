package router

import (
	"cc-052/internal/handler"
	"cc-052/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

func Setup(
	farmH *handler.FarmHandler,
	plotH *handler.PlotHandler,
	batchH *handler.BatchHandler,
	activityH *handler.ActivityHandler,
	inspectionH *handler.InspectionHandler,
	traceCodeH *handler.TraceCodeHandler,
	regionH *handler.RegionHandler,
	healthH *handler.HealthHandler,
	rdb *redis.Client,
) *gin.Engine {
	r := gin.Default()

	// Health check
	r.GET("/healthz", healthH.Health)

	// API v1
	v1 := r.Group("/api/v1")
	{
		// Farms / 合作社档案
		v1.POST("/farms", farmH.Create)
		v1.POST("/farms/cleanup", farmH.Cleanup) // 老数据归一化清洗（默认 dry_run）
		v1.GET("/farms", farmH.List)
		v1.GET("/farms/:id", farmH.GetByID)
		v1.PUT("/farms/:id", farmH.Update)
		v1.GET("/farms/:id/revisions", farmH.Revisions)

		// 资质问题：编号重复 / 已到期（给到期日）
		v1.GET("/reports/cert-issues", farmH.CertIssues)

		// 行政区划字典与老写法别名
		v1.GET("/regions", regionH.ListDict)
		v1.GET("/regions/aliases", regionH.ListAliases)
		v1.POST("/regions/aliases", regionH.UpsertAlias)

		// Plots
		v1.POST("/plots", plotH.Create)
		v1.GET("/plots/:id", plotH.GetByID)
		v1.GET("/plots", plotH.ListByFarm)

		// Batches
		v1.POST("/batches", batchH.Create)
		v1.GET("/batches/:id", batchH.GetByID)

		// Activities
		v1.POST("/batches/:id/activities", activityH.Create)
		v1.POST("/batches/:id/activities/batch", activityH.BatchCreate)
		v1.GET("/batches/:id/activities", activityH.ListByBatch)

		// Inspections
		v1.POST("/batches/:id/inspection", inspectionH.Create)

		// Trace codes
		v1.POST("/batches/:id/codes", traceCodeH.Generate)
	}

	// Public trace endpoints with rate limiting
	traceGroup := r.Group("/api/v1/trace")
	traceGroup.Use(middleware.RateLimit(rdb, 30, time.Minute))
	{
		traceGroup.GET("/:code", traceCodeH.Trace)
		traceGroup.GET("/:code/validate", traceCodeH.Validate)
	}

	return r
}
