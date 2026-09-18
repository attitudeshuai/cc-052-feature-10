package repository

import (
	"cc-052/internal/model"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

type FarmRepo struct {
	db *sqlx.DB
}

func NewFarmRepo(db *sqlx.DB) *FarmRepo {
	return &FarmRepo{db: db}
}

const farmColumns = `id, name, region_code, COALESCE(contact_ref,'') AS contact_ref,
	COALESCE(cert_no,'') AS cert_no, cert_expires_at,
	name_key, region_code_norm, cert_no_norm, created_at, updated_at`

// Create 插入档案，归一化列由服务层算好传入。
func (r *FarmRepo) Create(farm *model.Farm) error {
	query := `INSERT INTO farm
		(name, region_code, contact_ref, cert_no, cert_expires_at, name_key, region_code_norm, cert_no_norm)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRow(query,
		farm.Name, farm.RegionCode, farm.ContactRef, farm.CertNo, farm.CertExpiresAt,
		farm.NameKey, farm.RegionCodeNorm, farm.CertNoNorm,
	).Scan(&farm.ID, &farm.CreatedAt, &farm.UpdatedAt)
}

func (r *FarmRepo) GetByID(id int64) (*model.Farm, error) {
	var f model.Farm
	query := `SELECT ` + farmColumns + ` FROM farm WHERE id = $1`
	if err := r.db.Get(&f, query, id); err != nil {
		return nil, fmt.Errorf("farm not found: %w", err)
	}
	return &f, nil
}

func (r *FarmRepo) List() ([]model.Farm, error) {
	var farms []model.Farm
	query := `SELECT ` + farmColumns + ` FROM farm ORDER BY id`
	if err := r.db.Select(&farms, query); err != nil {
		return nil, err
	}
	return farms, nil
}

// NameRegionCandidates 返回判重候选：归一列精确命中的行 + 尚未回填归一列的老行。
// 老行由服务层在内存中归一后比对（老数据量随清洗回填逐渐归零）。
func (r *FarmRepo) NameRegionCandidates(regionNorm, nameKey string) ([]model.Farm, error) {
	var farms []model.Farm
	query := `SELECT ` + farmColumns + ` FROM farm
	          WHERE (region_code_norm = $1 AND name_key = $2)
	             OR name_key IS NULL OR region_code_norm IS NULL`
	if err := r.db.Select(&farms, query, regionNorm, nameKey); err != nil {
		return nil, err
	}
	return farms, nil
}

// CertCandidates 返回资质号判重候选：归一列命中的行 + 尚未回填的老行。
func (r *FarmRepo) CertCandidates(certNorm string) ([]model.Farm, error) {
	var farms []model.Farm
	query := `SELECT ` + farmColumns + ` FROM farm
	          WHERE cert_no_norm = $1 OR cert_no_norm IS NULL`
	if err := r.db.Select(&farms, query, certNorm); err != nil {
		return nil, err
	}
	return farms, nil
}

// ListAll 全表返回，批量清洗扫描用。
func (r *FarmRepo) ListAll() ([]model.Farm, error) {
	return r.List()
}

// allowedUpdateFields 白名单，防止动态 SQL 注入列名。
var allowedUpdateFields = map[string]bool{
	"name": true, "region_code": true, "contact_ref": true,
	"cert_no": true, "cert_expires_at": true,
	"name_key": true, "region_code_norm": true, "cert_no_norm": true,
}

// UpdateFields 按列白名单局部更新，返回更新后的完整档案。
func (r *FarmRepo) UpdateFields(id int64, fields map[string]interface{}) (*model.Farm, error) {
	if len(fields) == 0 {
		return r.GetByID(id)
	}
	cols := make([]string, 0, len(fields))
	for k := range fields {
		if !allowedUpdateFields[k] {
			return nil, fmt.Errorf("field not updatable: %s", k)
		}
		cols = append(cols, k)
	}
	sort.Strings(cols)

	setParts := make([]string, 0, len(cols)+1)
	args := make([]interface{}, 0, len(cols)+2)
	args = append(args, id)
	for i, c := range cols {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", c, i+2))
		args = append(args, fields[c])
	}
	setParts = append(setParts, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE farm SET %s WHERE id = $1", strings.Join(setParts, ", "))
	res, err := r.db.Exec(query, args...)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("farm not found: id=%d", id)
	}
	return r.GetByID(id)
}

func (r *FarmRepo) InsertRevision(rev *model.FarmRevision) error {
	query := `INSERT INTO farm_revision
		(farm_id, changed_fields, name_before, name_after,
		 region_code_before, region_code_after, cert_no_before, cert_no_after)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at`
	return r.db.QueryRow(query,
		rev.FarmID, rev.ChangedFields,
		rev.NameBefore, rev.NameAfter,
		rev.RegionCodeBefore, rev.RegionCodeAfter,
		rev.CertNoBefore, rev.CertNoAfter,
	).Scan(&rev.ID, &rev.CreatedAt)
}

func (r *FarmRepo) ListRevisions(farmID int64) ([]model.FarmRevision, error) {
	revs := []model.FarmRevision{}
	query := `SELECT id, farm_id, changed_fields, name_before, name_after,
		region_code_before, region_code_after, cert_no_before, cert_no_after, created_at
		FROM farm_revision WHERE farm_id = $1 ORDER BY id DESC`
	if err := r.db.Select(&revs, query, farmID); err != nil {
		return nil, err
	}
	return revs, nil
}

// WithTx 在一个事务里执行 fn（批量清洗 apply 用，保证要么全改、要么全回滚）。
func (r *FarmRepo) WithTx(fn func(tx *sqlx.Tx) error) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// TxUpdateFields 事务版 UpdateFields。
func (r *FarmRepo) TxUpdateFields(tx *sqlx.Tx, id int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	cols := make([]string, 0, len(fields))
	for k := range fields {
		if !allowedUpdateFields[k] {
			return fmt.Errorf("field not updatable: %s", k)
		}
		cols = append(cols, k)
	}
	sort.Strings(cols)

	setParts := make([]string, 0, len(cols)+1)
	args := make([]interface{}, 0, len(cols)+2)
	args = append(args, id)
	for i, c := range cols {
		setParts = append(setParts, fmt.Sprintf("%s = $%d", c, i+2))
		args = append(args, fields[c])
	}
	setParts = append(setParts, "updated_at = NOW()")

	query := fmt.Sprintf("UPDATE farm SET %s WHERE id = $1", strings.Join(setParts, ", "))
	res, err := tx.Exec(query, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("farm not found: id=%d", id)
	}
	return nil
}

// TxInsertRevision 事务版 InsertRevision。
func (r *FarmRepo) TxInsertRevision(tx *sqlx.Tx, rev *model.FarmRevision) error {
	query := `INSERT INTO farm_revision
		(farm_id, changed_fields, name_before, name_after,
		 region_code_before, region_code_after, cert_no_before, cert_no_after)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, created_at`
	return tx.QueryRow(query,
		rev.FarmID, rev.ChangedFields,
		rev.NameBefore, rev.NameAfter,
		rev.RegionCodeBefore, rev.RegionCodeAfter,
		rev.CertNoBefore, rev.CertNoAfter,
	).Scan(&rev.ID, &rev.CreatedAt)
}
