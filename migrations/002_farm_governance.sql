BEGIN;

-- Farm archive governance: normalized dedup keys, qualification expiry, change audit.
-- Idempotent: runs on every service boot together with 001_init.sql.

-- Normalized keys for duplicate detection (backfilled by the service at startup)
ALTER TABLE farm ADD COLUMN IF NOT EXISTS name_norm VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE farm ADD COLUMN IF NOT EXISTS region_norm VARCHAR(255) NOT NULL DEFAULT '';

-- Qualification expiry date (nullable for legacy rows; required for new archives)
ALTER TABLE farm ADD COLUMN IF NOT EXISTS cert_expires_at DATE;

CREATE INDEX IF NOT EXISTS idx_farm_norm ON farm(region_norm, name_norm);
CREATE INDEX IF NOT EXISTS idx_farm_cert_no ON farm(cert_no);

-- Audit trail: every name/region change keeps the before/after pair
CREATE TABLE IF NOT EXISTS farm_change_log (
    id BIGSERIAL PRIMARY KEY,
    farm_id BIGINT NOT NULL REFERENCES farm(id),
    field VARCHAR(32) NOT NULL CHECK (field IN ('name','region_code')),
    old_value TEXT NOT NULL,
    new_value TEXT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_farm_change_log_farm ON farm_change_log(farm_id);

COMMIT;
