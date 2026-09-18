package handler

import (
	"cc-052/internal/repository"
	"cc-052/pkg/response"
	"strconv"

	"github.com/gin-gonic/gin"
)

type RegionHandler struct {
	repo *repository.RegionRepo
}

func NewRegionHandler(repo *repository.RegionRepo) *RegionHandler {
	return &RegionHandler{repo: repo}
}

// ListDict GET /regions?level=3
func (h *RegionHandler) ListDict(c *gin.Context) {
	level := 0
	if l := c.Query("level"); l != "" {
		v, err := strconv.Atoi(l)
		if err != nil {
			response.BadRequest(c, "invalid level")
			return
		}
		level = v
	}
	dicts, err := h.repo.ListDict(level)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, dicts)
}

// ListAliases GET /regions/aliases
func (h *RegionHandler) ListAliases(c *gin.Context) {
	aliases, err := h.repo.ListAliases()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, aliases)
}

// UpsertAliasRequest 登记「老写法 -> 标准码」，供一次清洗归到同一取值。
type UpsertAliasRequest struct {
	Alias      string `json:"alias" binding:"required"`
	RegionCode string `json:"region_code" binding:"required"`
}

// UpsertAlias POST /regions/aliases
func (h *RegionHandler) UpsertAlias(c *gin.Context) {
	var req UpsertAliasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := h.repo.UpsertAlias(req.Alias, req.RegionCode); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Created(c, gin.H{"alias": req.Alias, "region_code": req.RegionCode})
}
