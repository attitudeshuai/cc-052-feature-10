package repository

import (
	"cc-052/internal/model"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type FarmRepo struct {
	db *sqlx.DB
}

func NewFarmRepo(db *sqlx.DB) *FarmRepo {
	return &FarmRepo{db: db}
}

const farmColumns = `id, name, region_code, contact_ref, cert_no, cert_expires_at, name_norm, region_norm, created_at`

func (r *FarmRepo) Create(farm *model.Farm) error {
	query := `INSERT INTO farm (name, region_code, contact_ref, cert_no, cert_expires_at, name_norm, region_norm)
	          VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at`
	return r.db.QueryRow(query, farm.Name, farm.RegionCode, farm.ContactRef,
		farm.CertNo, farm.CertExpiresAt, farm.NameNorm, farm.RegionNorm).
		Scan(&farm.ID, &farm.CreatedAt)
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

// FindByNorm returns farms matching the normalized (region, name) pair,
// oldest first. excludeID is skipped (pass 0 to match every row).
func (r *FarmRepo) FindByNorm(regionNorm, nameNorm string, excludeID int64) ([]model.Farm, error) {
	var farms []model.Farm
	query := `SELECT ` + farmColumns + ` FROM farm
	          WHERE region_norm = $1 AND name_norm = $2 AND id <> $3
	          ORDER BY id`
	if err := r.db.Select(&farms, query, regionNorm, nameNorm, excludeID); err != nil {
		return nil, err
	}
	return farms, nil
}

// UpdateWithLogs persists the farm and appends its name/region change logs
// in one transaction, so an archive edit never loses its audit trail.
func (r *FarmRepo) UpdateWithLogs(farm *model.Farm, logs []model.FarmChangeLog) error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	update := `UPDATE farm SET name = $1, region_code = $2, contact_ref = $3,
	           cert_no = $4, cert_expires_at = $5, name_norm = $6, region_norm = $7
	           WHERE id = $8`
	if _, err := tx.Exec(update, farm.Name, farm.RegionCode, farm.ContactRef,
		farm.CertNo, farm.CertExpiresAt, farm.NameNorm, farm.RegionNorm, farm.ID); err != nil {
		return err
	}

	insert := `INSERT INTO farm_change_log (farm_id, field, old_value, new_value) VALUES ($1, $2, $3, $4)`
	for i := range logs {
		if _, err := tx.Exec(insert, logs[i].FarmID, logs[i].Field, logs[i].OldValue, logs[i].NewValue); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateNorms refreshes only the normalized keys (legacy-data backfill).
func (r *FarmRepo) UpdateNorms(id int64, nameNorm, regionNorm string) error {
	query := `UPDATE farm SET name_norm = $1, region_norm = $2 WHERE id = $3`
	_, err := r.db.Exec(query, nameNorm, regionNorm, id)
	return err
}

// FieldChange records one row's current value before a batch normalize
// rewrites it to the canonical spelling.
type FieldChange struct {
	FarmID   int64
	OldValue string
}

// ApplyNormalize rewrites the given field to canonical for every change in a
// single transaction, logging each before/after pair, and returns the farm
// row count measured before and after inside the same transaction so callers
// can prove no row was lost or duplicated.
func (r *FarmRepo) ApplyNormalize(field, canonical, canonicalNorm string, changes []FieldChange) (before, after int64, err error) {
	var col, normCol string
	switch field {
	case "region_code":
		col, normCol = "region_code", "region_norm"
	case "name":
		col, normCol = "name", "name_norm"
	default:
		return 0, 0, fmt.Errorf("unsupported normalize field: %s", field)
	}

	tx, err := r.db.Beginx()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	if err = tx.Get(&before, `SELECT COUNT(*) FROM farm`); err != nil {
		return 0, 0, err
	}

	update := fmt.Sprintf(`UPDATE farm SET %s = $1, %s = $2 WHERE id = $3`, col, normCol)
	insert := `INSERT INTO farm_change_log (farm_id, field, old_value, new_value) VALUES ($1, $2, $3, $4)`
	for _, ch := range changes {
		if _, err = tx.Exec(update, canonical, canonicalNorm, ch.FarmID); err != nil {
			return 0, 0, err
		}
		if _, err = tx.Exec(insert, ch.FarmID, field, ch.OldValue, canonical); err != nil {
			return 0, 0, err
		}
	}

	if err = tx.Get(&after, `SELECT COUNT(*) FROM farm`); err != nil {
		return 0, 0, err
	}
	if before != after {
		// Row count changed mid-operation: roll everything back so the
		// archive never loses or gains rows silently.
		return 0, 0, fmt.Errorf("farm row count changed during normalize: %d -> %d, rolled back", before, after)
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return before, after, nil
}

// ListChanges returns the name/region change history of one farm, newest first.
func (r *FarmRepo) ListChanges(farmID int64) ([]model.FarmChangeLog, error) {
	var logs []model.FarmChangeLog
	query := `SELECT id, farm_id, field, old_value, new_value, changed_at
	          FROM farm_change_log WHERE farm_id = $1 ORDER BY changed_at DESC, id DESC`
	if err := r.db.Select(&logs, query, farmID); err != nil {
		return nil, err
	}
	return logs, nil
}
