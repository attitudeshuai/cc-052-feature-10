package model

// CertDuplicateGroup 同一个归一化资质编号被多家合作社占用。
type CertDuplicateGroup struct {
	CertNoNorm  string   `db:"cert_no_norm" json:"cert_no_norm"`
	Count       int      `db:"cnt" json:"count"`
	FarmIDs     []int64  `db:"farm_ids" json:"farm_ids"`
	Names       []string `db:"names" json:"names"`
	RegionCodes []string `db:"region_codes" json:"region_codes"`
	// RawCertNos 各家填写的原始编号（大小写/全半角可能不同）
	RawCertNos []string `db:"raw_cert_nos" json:"raw_cert_nos"`
}

// CertExpiredItem 资质已到期的合作社。
type CertExpiredItem struct {
	ID            int64  `db:"id" json:"id"`
	Name          string `db:"name" json:"name"`
	RegionCode    string `db:"region_code" json:"region_code"`
	CertNo        string `db:"cert_no" json:"cert_no,omitempty"`
	CertExpiresAt Date   `db:"cert_expires_at" json:"cert_expires_at"`
	DaysExpired   int    `db:"days_expired" json:"days_expired"`
}

// CertIssues 资质问题汇总。
type CertIssues struct {
	Duplicates []CertDuplicateGroup `json:"duplicates"`
	Expired    []CertExpiredItem    `json:"expired"`
}
