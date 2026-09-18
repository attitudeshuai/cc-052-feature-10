BEGIN;

-- ============================================================
-- 002: 合作社档案归一化 / 判重 / 资质 / 历史留痕
-- ============================================================

-- 归一化列（老数据先留 NULL，由清洗流程回填；存量允许重复/空值，判重在建档服务层做）
ALTER TABLE farm ADD COLUMN IF NOT EXISTS name_key          VARCHAR(255);
ALTER TABLE farm ADD COLUMN IF NOT EXISTS region_code_norm  VARCHAR(12);
ALTER TABLE farm ADD COLUMN IF NOT EXISTS cert_no_norm      VARCHAR(64);  -- 大写/去空白后比对，抓大小写变体
ALTER TABLE farm ADD COLUMN IF NOT EXISTS cert_expires_at   DATE;
ALTER TABLE farm ADD COLUMN IF NOT EXISTS updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- 查询用索引；不加 UNIQUE：存量脏数据里归一后可能重名/资质号重复，需由清洗流程消解
CREATE INDEX IF NOT EXISTS idx_farm_name_region ON farm(region_code_norm, name_key);
CREATE INDEX IF NOT EXISTS idx_farm_cert_norm    ON farm(cert_no_norm);

-- ============================================================
-- 行政区划字典（标准码 + 标准全称，村级可存 12 位）
-- ============================================================
CREATE TABLE IF NOT EXISTS region_dict (
    code        VARCHAR(12) PRIMARY KEY,
    full_name   VARCHAR(128) NOT NULL,
    short_name  VARCHAR(64)  NOT NULL,   -- 核心名，如「青龙」
    level       SMALLINT      NOT NULL,   -- 1省 2市 3县 4乡 5村
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 地区别名：老数据里的各种写法 -> 标准码
CREATE TABLE IF NOT EXISTS region_alias (
    id          BIGSERIAL PRIMARY KEY,
    alias       VARCHAR(128) NOT NULL,
    region_code VARCHAR(12)  NOT NULL REFERENCES region_dict(code),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_region_alias_alias ON region_alias(alias);

-- ============================================================
-- 档案改动历史：保留改动前后两版（名称 / 地区 / 资质）
-- ============================================================
CREATE TABLE IF NOT EXISTS farm_revision (
    id                    BIGSERIAL PRIMARY KEY,
    farm_id               BIGINT NOT NULL REFERENCES farm(id),
    changed_fields        TEXT[] NOT NULL,
    name_before           TEXT,
    name_after            TEXT,
    region_code_before    TEXT,
    region_code_after     TEXT,
    cert_no_before        TEXT,
    cert_no_after         TEXT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_farm_revision_farm ON farm_revision(farm_id, id);

COMMIT;
