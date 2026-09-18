package model

// CleanupChange 单行老数据的清洗结果（dry_run 与 apply 共用）。
type CleanupChange struct {
	FarmID int64        `json:"farm_id"`
	Name   string       `json:"name"`
	Before FarmSnapshot `json:"before"`
	After  FarmSnapshot `json:"after"`
	// Changes 实际发生变化的字段：region_code / name / cert_no
	Changes []string `json:"changes"`
}

// FarmSnapshot 用于改动前后对账的关键字段快照。
type FarmSnapshot struct {
	Name       string `json:"name"`
	RegionCode string `json:"region_code"`
	CertNo     string `json:"cert_no,omitempty"`
}

// CleanupCollision 归一后撞到同一（地区,名称）的多条档案，apply 时不会自动合并。
type CleanupCollision struct {
	RegionCodeNorm string         `json:"region_code_norm"`
	NameKey        string         `json:"name_key"`
	Farms          []FarmSnapshot `json:"farms"`
	FarmIDs        []int64        `json:"farm_ids"`
}

// CleanupResult 批量清洗结果与条数对账。
type CleanupResult struct {
	Applied bool `json:"applied"`

	TotalScanned  int `json:"total_scanned"`  // 改前总条数
	TotalAfter    int `json:"total_after"`    // 改后总条数（应与 scanned 相等）
	ChangedRows   int `json:"changed_rows"`   // 实际发生修改的行数
	UnchangedRows int `json:"unchanged_rows"` // 本来就规范、无需修改的行数

	RegionChanged int `json:"region_changed"` // 地区写法被归一的行数
	NameChanged   int `json:"name_changed"`   // 名称写法被归一的行数
	CertChanged   int `json:"cert_changed"`   // 资质号写法被归一的行数

	Changes    []CleanupChange     `json:"changes"`
	Unresolved []CleanupUnresolved `json:"unresolved"`
	Collisions []CleanupCollision  `json:"collisions"`
}

// CleanupUnresolved 地区无法识别、未做修改的行。
type CleanupUnresolved struct {
	FarmID     int64  `json:"farm_id"`
	Name       string `json:"name"`
	RegionCode string `json:"region_code"`
	Reason     string `json:"reason"`
}
