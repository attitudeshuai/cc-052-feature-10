package model

import (
	"time"

	"github.com/lib/pq"
)

type Farm struct {
	ID             int64     `db:"id" json:"id"`
	Name           string    `db:"name" json:"name"`
	RegionCode     string    `db:"region_code" json:"region_code"`
	ContactRef     string    `db:"contact_ref" json:"contact_ref,omitempty"`
	CertNo         string    `db:"cert_no" json:"cert_no,omitempty"`
	CertExpiresAt  *Date     `db:"cert_expires_at" json:"cert_expires_at,omitempty"`
	NameKey        *string   `db:"name_key" json:"-"`
	RegionCodeNorm *string   `db:"region_code_norm" json:"-"`
	CertNoNorm     *string   `db:"cert_no_norm" json:"-"`
	CreatedAt      time.Time `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time `db:"updated_at" json:"updated_at"`
}

// CreateFarmRequest 建档请求。
// 资质允许为空（空资质也能建档）；填写时建议同时给出 cert_expires_at。
type CreateFarmRequest struct {
	Name          string `json:"name" binding:"required"`
	RegionCode    string `json:"region_code" binding:"required"`
	ContactRef    string `json:"contact_ref"`
	CertNo        string `json:"cert_no"`
	CertExpiresAt string `json:"cert_expires_at"` // YYYY-MM-DD，可空
}

// UpdateFarmRequest 修改请求：nil 表示该字段不改，空串表示清空。
type UpdateFarmRequest struct {
	Name          *string `json:"name"`
	RegionCode    *string `json:"region_code"`
	ContactRef    *string `json:"contact_ref"`
	CertNo        *string `json:"cert_no"`
	CertExpiresAt *string `json:"cert_expires_at"` // YYYY-MM-DD；空串/null 清空
}

// FarmRevision 名称/地区/资质改动前后的两版留痕。
type FarmRevision struct {
	ID               int64          `db:"id" json:"id"`
	FarmID           int64          `db:"farm_id" json:"farm_id"`
	ChangedFields    pq.StringArray `db:"changed_fields" json:"changed_fields"`
	NameBefore       *string        `db:"name_before" json:"name_before"`
	NameAfter        *string        `db:"name_after" json:"name_after"`
	RegionCodeBefore *string        `db:"region_code_before" json:"region_code_before"`
	RegionCodeAfter  *string        `db:"region_code_after" json:"region_code_after"`
	CertNoBefore     *string        `db:"cert_no_before" json:"cert_no_before"`
	CertNoAfter      *string        `db:"cert_no_after" json:"cert_no_after"`
	CreatedAt        time.Time      `db:"created_at" json:"created_at"`
}

// RegionDict 行政区划标准条目。
type RegionDict struct {
	Code      string    `db:"code" json:"code"`
	FullName  string    `db:"full_name" json:"full_name"`
	ShortName string    `db:"short_name" json:"short_name"`
	Level     int16     `db:"level" json:"level"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// RegionAlias 地区别名（老写法 -> 标准码）。
type RegionAlias struct {
	ID         int64     `db:"id" json:"id"`
	Alias      string    `db:"alias" json:"alias"`
	RegionCode string    `db:"region_code" json:"region_code"`
	CreatedAt  time.Time `db:"created_at" json:"created_at"`
}
