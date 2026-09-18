package handler

import (
	"cc-052/internal/model"
	"cc-052/internal/service"
	"cc-052/pkg/response"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FarmHandler struct {
	svc *service.FarmService
}

func NewFarmHandler(svc *service.FarmService) *FarmHandler {
	return &FarmHandler{svc: svc}
}

func (h *FarmHandler) Create(c *gin.Context) {
	var req model.CreateFarmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	res, err := h.svc.Create(&req)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	response.Created(c, res)
}

func (h *FarmHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var req model.UpdateFarmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	res, err := h.svc.Update(id, &req)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	response.Success(c, res)
}

func (h *FarmHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	farm, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "farm not found")
		return
	}
	response.Success(c, farm)
}

func (h *FarmHandler) List(c *gin.Context) {
	farms, err := h.svc.List()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, farms)
}

func (h *FarmHandler) Revisions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	revs, err := h.svc.ListRevisions(id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, revs)
}

// CertIssues 资质编号重复 / 已到期清单。
func (h *FarmHandler) CertIssues(c *gin.Context) {
	issues, err := h.svc.CertIssues()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, issues)
}

// Cleanup 老数据归一化清洗。默认 dry_run 只出方案，
// 显式 apply=true（query 或 body）才落库。
func (h *FarmHandler) Cleanup(c *gin.Context) {
	apply := c.Query("apply") == "true" || c.Query("dry_run") == "false"
	if !apply {
		var body struct {
			Apply bool `json:"apply"`
		}
		// body 可缺省
		_ = c.ShouldBindJSON(&body)
		apply = body.Apply
	}
	res, err := h.svc.Cleanup(!apply)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, res)
}

func (h *FarmHandler) writeServiceError(c *gin.Context, err error) {
	var ce *service.ConflictError
	if errors.As(err, &ce) {
		response.Conflict(c, ce.Message, gin.H{
			"kind":     ce.Kind,
			"existing": ce.Existing,
		})
		return
	}
	var ve *service.ValidationError
	if errors.As(err, &ve) {
		response.BadRequest(c, ve.Message)
		return
	}
	response.InternalError(c, err.Error())
}
