package model

import "time"

type Farm struct {
	ID            int64      `db:"id" json:"id"`
	Name          string     `db:"name" json:"name"`
	RegionCode    string     `db:"region_code" json:"region_code"`
	ContactRef    string     `db:"contact_ref" json:"contact_ref"`
	CertNo        string     `db:"cert_no" json:"cert_no"`
	CertExpiresAt *time.Time `db:"cert_expires_at" json:"cert_expires_at,omitempty"`
	NameNorm      string     `db:"name_norm" json:"-"`
	RegionNorm    string     `db:"region_norm" json:"-"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

type CreateFarmRequest struct {
	Name          string `json:"name" binding:"required"`
	RegionCode    string `json:"region_code" binding:"required"`
	ContactRef    string `json:"contact_ref"`
	CertNo        string `json:"cert_no" binding:"required"`
	CertExpiresAt string `json:"cert_expires_at" binding:"required"` // YYYY-MM-DD
}

// UpdateFarmRequest uses pointers so omitted fields stay untouched.
type UpdateFarmRequest struct {
	Name          *string `json:"name"`
	RegionCode    *string `json:"region_code"`
	ContactRef    *string `json:"contact_ref"`
	CertNo        *string `json:"cert_no"`
	CertExpiresAt *string `json:"cert_expires_at"` // YYYY-MM-DD
}

// FarmChangeLog keeps the before/after pair of one name/region change.
type FarmChangeLog struct {
	ID        int64     `db:"id" json:"id"`
	FarmID    int64     `db:"farm_id" json:"farm_id"`
	Field     string    `db:"field" json:"field"` // "name" or "region_code"
	OldValue  string    `db:"old_value" json:"old_value"`
	NewValue  string    `db:"new_value" json:"new_value"`
	ChangedAt time.Time `db:"changed_at" json:"changed_at"`
}

// NormalizeFieldRequest merges several legacy spellings of one field into a
// single canonical value in one shot.
type NormalizeFieldRequest struct {
	Field     string   `json:"field" binding:"required"`     // "region_code" or "name"
	Canonical string   `json:"canonical" binding:"required"` // target value
	Aliases   []string `json:"aliases" binding:"required"`   // legacy spellings to rewrite
}

// NormalizeFieldResult reports how many rows were rewritten and proves the
// total row count is unchanged by the operation.
type NormalizeFieldResult struct {
	Field       string `json:"field"`
	Canonical   string `json:"canonical"`
	Updated     int    `json:"updated"`
	TotalBefore int64  `json:"total_before"`
	TotalAfter  int64  `json:"total_after"`
	CountsMatch bool   `json:"counts_match"`
}

// CertIssueFarm is one farm entry inside the qualification issue report.
type CertIssueFarm struct {
	FarmID        int64  `json:"farm_id"`
	Name          string `json:"name"`
	RegionCode    string `json:"region_code"`
	CertNo        string `json:"cert_no"`
	CertExpiresAt string `json:"cert_expires_at,omitempty"` // YYYY-MM-DD
}

// DuplicateCertGroup groups farms sharing the same normalized cert number.
type DuplicateCertGroup struct {
	CertNo string          `json:"cert_no"`
	Count  int             `json:"count"`
	Farms  []CertIssueFarm `json:"farms"`
}

// CertIssuesReport lists duplicate and expired qualifications separately.
type CertIssuesReport struct {
	Duplicates []DuplicateCertGroup `json:"duplicates"`
	Expired    []CertIssueFarm      `json:"expired"`
}
