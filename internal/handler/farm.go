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

// writeServiceError maps typed service errors to HTTP statuses.
func writeServiceError(c *gin.Context, err error) {
	var conflict *service.ConflictError
	var invalid *service.ValidationError
	var notFound *service.NotFoundError
	switch {
	case errors.As(err, &conflict):
		response.Conflict(c, conflict.Error(), conflict.Existing)
	case errors.As(err, &invalid):
		response.BadRequest(c, invalid.Error())
	case errors.As(err, &notFound):
		response.NotFound(c, notFound.Error())
	default:
		response.InternalError(c, err.Error())
	}
}

func (h *FarmHandler) Create(c *gin.Context) {
	var req model.CreateFarmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	farm, err := h.svc.Create(&req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Created(c, farm)
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

// Update partially edits a farm; name/region changes are audited.
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
	farm, err := h.svc.Update(id, &req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, farm)
}

// Changes returns the before/after history of name/region edits.
func (h *FarmHandler) Changes(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	logs, err := h.svc.Changes(id)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, logs)
}

// CertIssues lists duplicate and expired qualification numbers.
func (h *FarmHandler) CertIssues(c *gin.Context) {
	report, err := h.svc.CertIssues()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, report)
}

// Normalize merges legacy spellings of region/name into one canonical value.
func (h *FarmHandler) Normalize(c *gin.Context) {
	var req model.NormalizeFieldRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.svc.NormalizeField(&req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.Success(c, result)
}
