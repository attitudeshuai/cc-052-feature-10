package repository

import (
	"cc-052/internal/model"
	"cc-052/pkg/normalize"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type RegionRepo struct {
	db *sqlx.DB
}

func NewRegionRepo(db *sqlx.DB) *RegionRepo {
	return &RegionRepo{db: db}
}

// BatchResolve 批量解析地区输入，返回 rawInput -> 标准码（未识别的不在 map 中）。
// 匹配优先级：别名/标准全称/简称（精确归一） > 长数字码前缀到县 > 标准名核心名兜底。
// 字典量级很小，一次性载入内存比对，避免逐行查询。
func (r *RegionRepo) BatchResolve(raws []string) (map[string]string, error) {
	out := make(map[string]string, len(raws))
	if len(raws) == 0 {
		return out, nil
	}

	uniq := make(map[string]struct{}, len(raws))
	for _, v := range raws {
		if v != "" {
			uniq[v] = struct{}{}
		}
	}
	if len(uniq) == 0 {
		return out, nil
	}

	var aliases []model.RegionAlias
	if err := r.db.Select(&aliases,
		`SELECT id, alias, region_code, created_at FROM region_alias`); err != nil {
		return nil, err
	}
	var dicts []model.RegionDict
	if err := r.db.Select(&dicts,
		`SELECT code, full_name, short_name, level, updated_at FROM region_dict`); err != nil {
		return nil, err
	}

	exact := make(map[string]string)    // 高优先级：别名、全称、简称、标准码
	fallback := make(map[string]string) // 低优先级：核心名
	putExact := func(k, code string) {
		if k == "" {
			return
		}
		if _, ok := exact[k]; !ok {
			exact[k] = code
		}
	}
	for _, a := range aliases {
		putExact(normalize.Text(a.Alias), a.RegionCode)
	}
	for _, d := range dicts {
		putExact(normalize.Text(d.FullName), d.Code)
		putExact(normalize.Text(d.ShortName), d.Code)
		putExact(d.Code, d.Code)
		core := normalize.CoreRegionName(d.FullName)
		if core != "" {
			if _, ok := fallback[core]; !ok {
				fallback[core] = d.Code
			}
		}
	}

	for v := range uniq {
		key := normalize.RegionCodeInput(v)
		if code, ok := exact[key]; ok {
			out[v] = code
			continue
		}
		if code, ok := exact[normalize.Text(v)]; ok {
			out[v] = code
			continue
		}
		// 「青龙满族县」这类没进别名表的变体：剥成核心名兜底
		if code, ok := fallback[normalize.CoreRegionName(v)]; ok {
			out[v] = code
		}
	}
	return out, nil
}

// UpsertAlias 新增或更新一条「老写法 -> 标准码」别名，标准码必须存在。
func (r *RegionRepo) UpsertAlias(alias, regionCode string) error {
	var exists int
	if err := r.db.Get(&exists, `SELECT 1 FROM region_dict WHERE code = $1`, regionCode); err != nil {
		return fmt.Errorf("region code not found in dict: %s", regionCode)
	}
	_, err := r.db.Exec(`
		INSERT INTO region_alias (alias, region_code) VALUES ($1, $2)
		ON CONFLICT (alias) DO UPDATE SET region_code = EXCLUDED.region_code`,
		alias, regionCode)
	return err
}

// ListDict 返回字典（level<=0 表示全部层级）。
func (r *RegionRepo) ListDict(level int) ([]model.RegionDict, error) {
	var dicts []model.RegionDict
	q := `SELECT code, full_name, short_name, level, updated_at FROM region_dict`
	if level > 0 {
		q += ` WHERE level = $1`
	}
	q += ` ORDER BY code`
	var err error
	if level > 0 {
		err = r.db.Select(&dicts, q, level)
	} else {
		err = r.db.Select(&dicts, q)
	}
	if err != nil {
		return nil, err
	}
	return dicts, nil
}

// ListAliases 返回全部别名。
func (r *RegionRepo) ListAliases() ([]model.RegionAlias, error) {
	var aliases []model.RegionAlias
	q := `SELECT id, alias, region_code, created_at FROM region_alias ORDER BY alias`
	if err := r.db.Select(&aliases, q); err != nil {
		return nil, err
	}
	return aliases, nil
}
