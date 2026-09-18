package service

import (
	"cc-052/internal/model"
	"cc-052/internal/repository"
	"cc-052/pkg/normalize"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type FarmService struct {
	repo   *repository.FarmRepo
	region *repository.RegionRepo
	now    func() time.Time // 可在测试中替换
}

func NewFarmService(repo *repository.FarmRepo, regionRepo *repository.RegionRepo) *FarmService {
	return &FarmService{repo: repo, region: regionRepo, now: time.Now}
}

// CreateResult 建档结果：档案本体 + 非阻断性提示（如资质已到期）。
type CreateResult struct {
	Farm     *model.Farm `json:"farm"`
	Warnings []string    `json:"warnings,omitempty"`
}

// canonicalDisplay 把名称整理成统一展示写法：全角折半角、空白压缩为单个半角空格并去首尾空白。
func canonicalDisplay(raw string) string {
	var b strings.Builder
	prevSpace := false
	ended := true // 用于去首尾空白
	for _, r := range raw {
		if r == 0x3000 {
			r = ' '
		} else if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}
		if isSpaceRune(r) {
			if !ended && !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
		ended = false
	}
	return strings.TrimRight(b.String(), " ")
}

func isSpaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == 0x3000
}

func strPtr(s string) *string { return &s }

// parseExpiry 解析资质到期日；空串返回 nil。
func parseExpiry(s string) (*model.Date, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	d, err := model.ParseDate(s)
	if err != nil {
		return nil, &ValidationError{Message: "cert_expires_at 格式应为 YYYY-MM-DD"}
	}
	return &d, nil
}

// Create 建档：归一地区 → 归一名称判重（指出已存在的是哪一家）→ 资质号判重 → 到期仅告警。
// 空资质允许建档。
func (s *FarmService) Create(req *model.CreateFarmRequest) (*CreateResult, error) {
	displayName := canonicalDisplay(req.Name)
	if displayName == "" {
		return nil, &ValidationError{Message: "name 不能为空"}
	}
	nameKey := normalize.NameKey(req.Name)

	resolved, err := s.region.BatchResolve([]string{req.RegionCode})
	if err != nil {
		return nil, fmt.Errorf("resolve region: %w", err)
	}
	regionNorm, ok := resolved[req.RegionCode]
	if !ok {
		return nil, &ValidationError{Message: fmt.Sprintf(
			"无法识别的地区编码/名称 %q，请使用标准行政区划码或先登记别名", req.RegionCode)}
	}

	certNorm := normalize.CertNo(req.CertNo)
	certDisplay := certNorm // 统一按大写无空白的写法存档，全角/小写变体一次改到同一取值

	expiry, err := parseExpiry(req.CertExpiresAt)
	if err != nil {
		return nil, err
	}

	// 1) 同地区 + 归一名称 判重
	if dup, err := s.findNameDup(regionNorm, nameKey, 0, resolved); err != nil {
		return nil, err
	} else if dup != nil {
		return nil, &ConflictError{
			Kind: "duplicate_name",
			Message: fmt.Sprintf(
				"归一化后与已存在档案重名（地区 %s / 名称键 %s），已存在：id=%d %q（%s）",
				regionNorm, nameKey, dup.ID, dup.Name, dup.RegionCode),
			Existing: dup,
		}
	}

	// 2) 资质号占用判重（空资质跳过）
	if certNorm != "" {
		if dup, err := s.findCertDup(certNorm, 0); err != nil {
			return nil, err
		} else if dup != nil {
			return nil, &ConflictError{
				Kind: "duplicate_cert",
				Message: fmt.Sprintf(
					"资质编号 %q 已被另一家合作社占用：id=%d %q（%s，到期日 %s）",
					certNorm, dup.ID, dup.Name, dup.RegionCode, expiryText(dup.CertExpiresAt)),
				Existing: dup,
			}
		}
	}

	farm := &model.Farm{
		Name:           displayName,
		RegionCode:     regionNorm, // 新档案直接存标准码
		ContactRef:     strings.TrimSpace(req.ContactRef),
		CertNo:         certDisplay,
		CertExpiresAt:  expiry,
		NameKey:        strPtr(nameKey),
		RegionCodeNorm: strPtr(regionNorm),
	}
	if certNorm == "" {
		farm.CertNoNorm = nil
	} else {
		farm.CertNoNorm = strPtr(certNorm)
	}

	if err := s.repo.Create(farm); err != nil {
		return nil, fmt.Errorf("create farm: %w", err)
	}

	return &CreateResult{Farm: farm, Warnings: s.farmWarnings(farm)}, nil
}

func expiryText(d *model.Date) string {
	if d == nil || d.Time.IsZero() {
		return "未登记"
	}
	return d.Format()
}

// farmWarnings 资质到期相关的非阻断提示。
func (s *FarmService) farmWarnings(f *model.Farm) []string {
	if f.CertExpiresAt == nil || f.CertExpiresAt.Time.IsZero() {
		return nil
	}
	today := model.NewDate(s.now())
	if f.CertExpiresAt.Time.Before(today.Time) {
		days := int(today.Time.Sub(f.CertExpiresAt.Time).Hours() / 24)
		return []string{fmt.Sprintf("资质编号 %s 已于 %s 到期（已过期 %d 天），档案仍可建立",
			f.CertNo, f.CertExpiresAt.Format(), days)}
	}
	return nil
}

// findNameDup 在候选行中找归一后同地区同名的档案，excludeID 用于修改时排除自身。
// resolvedSoFar 可复用上层已批量解析过的结果（key 为原始地区写法）。
func (s *FarmService) findNameDup(regionNorm, nameKey string, excludeID int64,
	resolvedSoFar map[string]string) (*model.Farm, error) {

	candidates, err := s.repo.NameRegionCandidates(regionNorm, nameKey)
	if err != nil {
		return nil, err
	}
	// 补解析候选老行的原始地区写法
	need := map[string]struct{}{}
	for _, c := range candidates {
		if _, ok := resolvedSoFar[c.RegionCode]; !ok {
			need[c.RegionCode] = struct{}{}
		}
	}
	resolved := resolvedSoFar
	if len(need) > 0 {
		raws := make([]string, 0, len(need))
		for r := range need {
			raws = append(raws, r)
		}
		more, err := s.region.BatchResolve(raws)
		if err != nil {
			return nil, err
		}
		resolved = make(map[string]string, len(resolvedSoFar)+len(more))
		for k, v := range resolvedSoFar {
			resolved[k] = v
		}
		for k, v := range more {
			resolved[k] = v
		}
	}
	for i := range candidates {
		c := &candidates[i]
		if c.ID == excludeID {
			continue
		}
		cRegion, ok := resolved[c.RegionCode]
		if !ok {
			continue
		}
		key := c.NameKey
		if key == nil {
			k := normalize.NameKey(c.Name)
			key = &k
		}
		rn := c.RegionCodeNorm
		if rn == nil {
			r := cRegion
			rn = &r
		}
		if *rn == regionNorm && *key == nameKey {
			return c, nil
		}
	}
	return nil, nil
}

// findCertDup 在候选行中找归一后资质号相同的档案。
func (s *FarmService) findCertDup(certNorm string, excludeID int64) (*model.Farm, error) {
	candidates, err := s.repo.CertCandidates(certNorm)
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		c := &candidates[i]
		if c.ID == excludeID {
			continue
		}
		norm := c.CertNoNorm
		if norm == nil {
			n := normalize.CertNo(c.CertNo)
			norm = &n
		}
		if *norm == certNorm {
			return c, nil
		}
	}
	return nil, nil
}

func (s *FarmService) GetByID(id int64) (*model.Farm, error) {
	return s.repo.GetByID(id)
}

func (s *FarmService) List() ([]model.Farm, error) {
	return s.repo.List()
}

func (s *FarmService) ListRevisions(farmID int64) ([]model.FarmRevision, error) {
	return s.repo.ListRevisions(farmID)
}

// Update 修改档案；名称/地区/资质发生变化时写入改动前后两版的 revision。
func (s *FarmService) Update(id int64, req *model.UpdateFarmRequest) (*CreateResult, error) {
	farm, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	before := model.FarmSnapshot{Name: farm.Name, RegionCode: farm.RegionCode, CertNo: farm.CertNo}
	fields := map[string]interface{}{}

	newName := farm.Name
	newNameKey := farm.NameKey
	if req.Name != nil {
		newName = canonicalDisplay(*req.Name)
		if newName == "" {
			return nil, &ValidationError{Message: "name 不能为空"}
		}
		k := normalize.NameKey(*req.Name)
		newNameKey = &k
	}

	newRegion := farm.RegionCode
	newRegionNorm := farm.RegionCodeNorm
	if req.RegionCode != nil {
		resolved, err := s.region.BatchResolve([]string{*req.RegionCode})
		if err != nil {
			return nil, fmt.Errorf("resolve region: %w", err)
		}
		code, ok := resolved[*req.RegionCode]
		if !ok {
			return nil, &ValidationError{Message: fmt.Sprintf(
				"无法识别的地区编码/名称 %q，请使用标准行政区划码或先登记别名", *req.RegionCode)}
		}
		newRegion = code
		newRegionNorm = &code
	}

	newCert := farm.CertNo
	newCertNorm := farm.CertNoNorm
	newExpiry := farm.CertExpiresAt
	if req.CertNo != nil {
		newCert = normalize.CertNo(*req.CertNo)
		if newCert != "" {
			newCertNorm = &newCert
		} else {
			newCertNorm = nil
		}
	}
	if req.CertExpiresAt != nil {
		d, err := parseExpiry(*req.CertExpiresAt)
		if err != nil {
			return nil, err
		}
		newExpiry = d
	}

	// 本次改动后该行生效的归一值；老档案归一列可能还是 NULL，需要现算兜底，
	// 否则只改名称（地区列未回填）时会漏掉判重。
	effRegionNorm := newRegionNorm
	if effRegionNorm == nil {
		resolved, err := s.region.BatchResolve([]string{newRegion})
		if err != nil {
			return nil, fmt.Errorf("resolve region: %w", err)
		}
		if code, ok := resolved[newRegion]; ok {
			effRegionNorm = &code
		}
	}
	effNameKey := newNameKey
	if effNameKey == nil {
		k := normalize.NameKey(newName)
		effNameKey = &k
	}

	// 归一冲突检查（排除自身）
	if (req.Name != nil || req.RegionCode != nil) &&
		effRegionNorm != nil && effNameKey != nil {
		dup, err := s.findNameDup(*effRegionNorm, *effNameKey, farm.ID, nil)
		if err != nil {
			return nil, err
		}
		if dup != nil {
			return nil, &ConflictError{
				Kind: "duplicate_name",
				Message: fmt.Sprintf(
					"修改后归一化名称与已存在档案重名：id=%d %q（%s）",
					dup.ID, dup.Name, dup.RegionCode),
				Existing: dup,
			}
		}
	}
	if req.CertNo != nil && newCertNorm != nil {
		dup, err := s.findCertDup(*newCertNorm, farm.ID)
		if err != nil {
			return nil, err
		}
		if dup != nil {
			return nil, &ConflictError{
				Kind: "duplicate_cert",
				Message: fmt.Sprintf(
					"资质编号 %q 已被另一家合作社占用：id=%d %q（%s）",
					*newCertNorm, dup.ID, dup.Name, dup.RegionCode),
				Existing: dup,
			}
		}
	}

	if req.Name != nil {
		fields["name"] = newName
		fields["name_key"] = newNameKey
	}
	if req.RegionCode != nil {
		fields["region_code"] = newRegion
		fields["region_code_norm"] = newRegionNorm
	}
	if req.ContactRef != nil {
		fields["contact_ref"] = strings.TrimSpace(*req.ContactRef)
	}
	if req.CertNo != nil {
		fields["cert_no"] = newCert
		fields["cert_no_norm"] = newCertNorm
	}
	if req.CertExpiresAt != nil {
		fields["cert_expires_at"] = newExpiry
	}

	changedTracked := []string{}
	if req.Name != nil && before.Name != newName {
		changedTracked = append(changedTracked, "name")
	}
	if req.RegionCode != nil && before.RegionCode != newRegion {
		changedTracked = append(changedTracked, "region_code")
	}
	if req.CertNo != nil && before.CertNo != newCert {
		changedTracked = append(changedTracked, "cert_no")
	}

	var updated *model.Farm
	err = s.repo.WithTx(func(tx *sqlx.Tx) error {
		if err := s.repo.TxUpdateFields(tx, id, fields); err != nil {
			return err
		}
		if len(changedTracked) > 0 {
			rev := &model.FarmRevision{
				FarmID:        id,
				ChangedFields: pq.StringArray(changedTracked),
				NameBefore:    strPtr(before.Name),
				NameAfter:     strPtr(newName),
			}
			rev.RegionCodeBefore = strPtr(before.RegionCode)
			rev.RegionCodeAfter = strPtr(newRegion)
			rev.CertNoBefore = strPtr(before.CertNo)
			rev.CertNoAfter = strPtr(newCert)
			if err := s.repo.TxInsertRevision(tx, rev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	updated, err = s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return &CreateResult{Farm: updated, Warnings: s.farmWarnings(updated)}, nil
}

// ============================================================
// 批量清洗：老数据一次改到同一取值 + 前后条数对账
// ============================================================

// canonicalNameOf 在一个重名组内挑选统一展示名称：
// 优先含正式组织形式后缀的写法，后缀越长越优先；并列取 id 最小（确定性）。
func canonicalNameOf(members []model.Farm) string {
	best := members[0]
	bestScore := suffixScore(best.Name)
	for _, m := range members[1:] {
		sc := suffixScore(m.Name)
		if sc > bestScore || (sc == bestScore && m.ID < best.ID) {
			best, bestScore = m, sc
		}
	}
	return canonicalDisplay(best.Name)
}

func suffixScore(name string) int {
	k := normalize.NameKey(name)
	n := normalize.Text(name)
	score := len([]rune(n)) - len([]rune(k)) // 被剥掉的后缀越长分越高
	return score
}

// Cleanup dryRun=true 只出方案；apply=false 不写库。
func (s *FarmService) Cleanup(dryRun bool) (*model.CleanupResult, error) {
	farms, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}

	raws := make([]string, 0, len(farms))
	seen := map[string]struct{}{}
	for _, f := range farms {
		if _, ok := seen[f.RegionCode]; !ok {
			seen[f.RegionCode] = struct{}{}
			raws = append(raws, f.RegionCode)
		}
	}
	resolved, err := s.region.BatchResolve(raws)
	if err != nil {
		return nil, err
	}

	// 按归一（地区, 名称键）分组，挑组内统一名称
	type groupKey struct{ region, key string }
	groups := map[groupKey][]int{} // farms 下标
	type planRow struct {
		farm       *model.Farm
		newName    string
		newRegion  string
		newCert    string
		changes    []string
		unresolved bool
	}
	plans := make([]planRow, 0, len(farms))

	// 先算每行的 key 与分组
	keys := make([]groupKey, len(farms))
	for i := range farms {
		f := &farms[i]
		region, ok := resolved[f.RegionCode]
		if !ok {
			keys[i] = groupKey{} // 未识别，不参与分组
			continue
		}
		k := normalize.NameKey(f.Name)
		keys[i] = groupKey{region: region, key: k}
		groups[keys[i]] = append(groups[keys[i]], i)
	}

	result := &model.CleanupResult{Applied: !dryRun, TotalScanned: len(farms)}

	for i := range farms {
		f := &farms[i]
		before := model.FarmSnapshot{Name: f.Name, RegionCode: f.RegionCode, CertNo: f.CertNo}

		region, ok := resolved[f.RegionCode]
		if !ok {
			result.Unresolved = append(result.Unresolved, model.CleanupUnresolved{
				FarmID:     f.ID,
				Name:       f.Name,
				RegionCode: f.RegionCode,
				Reason:     "地区写法无法在字典/别名中识别，已跳过；请登记别名后重新清洗",
			})
			plans = append(plans, planRow{farm: f, unresolved: true})
			continue
		}

		memberIdxs := groups[keys[i]]
		members := make([]model.Farm, 0, len(memberIdxs))
		for _, mi := range memberIdxs {
			members = append(members, farms[mi])
		}
		newName := canonicalNameOf(members)
		newCert := normalize.CertNo(f.CertNo)

		changes := []string{}
		if newRegion := region; f.RegionCode != newRegion {
			changes = append(changes, "region_code")
		}
		if newName != canonicalDisplay(f.Name) {
			changes = append(changes, "name")
		}
		if f.CertNo != "" && newCert != f.CertNo {
			changes = append(changes, "cert_no")
		}
		plans = append(plans, planRow{
			farm:      f,
			newName:   newName,
			newRegion: region,
			newCert:   newCert,
			changes:   changes,
		})

		after := model.FarmSnapshot{Name: newName, RegionCode: region, CertNo: newCert}
		if len(changes) > 0 {
			result.Changes = append(result.Changes, model.CleanupChange{
				FarmID:  f.ID,
				Name:    f.Name,
				Before:  before,
				After:   after,
				Changes: changes,
			})
			for _, c := range changes {
				switch c {
				case "region_code":
					result.RegionChanged++
				case "name":
					result.NameChanged++
				case "cert_no":
					result.CertChanged++
				}
			}
		}
	}

	// 归一后重名碰撞组（>1 家），交人工决定是否合并，清洗不自动删行
	for gk, idxs := range groups {
		if len(idxs) <= 1 {
			continue
		}
		coll := model.CleanupCollision{
			RegionCodeNorm: gk.region,
			NameKey:        gk.key,
		}
		for _, mi := range idxs {
			f := &farms[mi]
			coll.Farms = append(coll.Farms, model.FarmSnapshot{
				Name: f.Name, RegionCode: f.RegionCode, CertNo: f.CertNo,
			})
			coll.FarmIDs = append(coll.FarmIDs, f.ID)
		}
		sort.Slice(coll.FarmIDs, func(a, b int) bool { return coll.FarmIDs[a] < coll.FarmIDs[b] })
		result.Collisions = append(result.Collisions, coll)
	}
	sort.Slice(result.Collisions, func(a, b int) bool {
		if result.Collisions[a].RegionCodeNorm != result.Collisions[b].RegionCodeNorm {
			return result.Collisions[a].RegionCodeNorm < result.Collisions[b].RegionCodeNorm
		}
		return result.Collisions[a].NameKey < result.Collisions[b].NameKey
	})

	result.ChangedRows = len(result.Changes)
	result.UnchangedRows = result.TotalScanned - result.ChangedRows - len(result.Unresolved)

	if dryRun {
		result.TotalAfter = result.TotalScanned // 预演不落库，条数必然不变
		return result, nil
	}

	// apply：单事务逐行更新 + 留痕，要么全成要么全回滚
	err = s.repo.WithTx(func(tx *sqlx.Tx) error {
		for _, p := range plans {
			if p.unresolved || len(p.changes) == 0 {
				continue
			}
			f := p.farm
			fields := map[string]interface{}{
				"region_code":      p.newRegion,
				"region_code_norm": p.newRegion,
				"name":             p.newName,
				"name_key":         normalize.NameKey(p.newName),
			}
			if f.CertNo != "" {
				fields["cert_no"] = p.newCert
				fields["cert_no_norm"] = normalize.CertNo(p.newCert)
			} else {
				// 原本就没资质：补 NULL 归一列，保持空资质可建档
				fields["cert_no_norm"] = nil
			}
			if err := s.repo.TxUpdateFields(tx, f.ID, fields); err != nil {
				return err
			}
			rev := &model.FarmRevision{
				FarmID:           f.ID,
				ChangedFields:    pq.StringArray(p.changes),
				NameBefore:       strPtr(f.Name),
				NameAfter:        strPtr(p.newName),
				RegionCodeBefore: strPtr(f.RegionCode),
				RegionCodeAfter:  strPtr(p.newRegion),
				CertNoBefore:     strPtr(f.CertNo),
				CertNoAfter:      strPtr(p.newCert),
			}
			if err := s.repo.TxInsertRevision(tx, rev); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("apply cleanup: %w", err)
	}

	// 改完对账：总条数必须与改前一致
	afterFarms, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}
	result.TotalAfter = len(afterFarms)
	if result.TotalAfter != result.TotalScanned {
		return result, fmt.Errorf("对账失败：清洗前 %d 条，清洗后 %d 条",
			result.TotalScanned, result.TotalAfter)
	}
	return result, nil
}

// ============================================================
// 资质问题报表：重复编号 + 已到期（给到期日）
// ============================================================

func (s *FarmService) CertIssues() (*model.CertIssues, error) {
	farms, err := s.repo.ListAll()
	if err != nil {
		return nil, err
	}

	groups := map[string][]model.Farm{}
	for _, f := range farms {
		n := normalize.CertNo(f.CertNo)
		if n == "" {
			continue
		}
		groups[n] = append(groups[n], f)
	}
	keys := make([]string, 0, len(groups))
	for k, v := range groups {
		if len(v) > 1 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	issues := &model.CertIssues{Duplicates: []model.CertDuplicateGroup{}, Expired: []model.CertExpiredItem{}}
	for _, k := range keys {
		members := groups[k]
		sort.Slice(members, func(a, b int) bool { return members[a].ID < members[b].ID })
		g := model.CertDuplicateGroup{CertNoNorm: k, Count: len(members)}
		for _, m := range members {
			g.FarmIDs = append(g.FarmIDs, m.ID)
			g.Names = append(g.Names, m.Name)
			g.RegionCodes = append(g.RegionCodes, m.RegionCode)
			g.RawCertNos = append(g.RawCertNos, m.CertNo)
		}
		issues.Duplicates = append(issues.Duplicates, g)
	}

	today := model.NewDate(s.now())
	for _, f := range farms {
		if normalize.CertNo(f.CertNo) == "" || f.CertExpiresAt == nil || f.CertExpiresAt.Time.IsZero() {
			continue
		}
		if f.CertExpiresAt.Time.Before(today.Time) {
			issues.Expired = append(issues.Expired, model.CertExpiredItem{
				ID:            f.ID,
				Name:          f.Name,
				RegionCode:    f.RegionCode,
				CertNo:        f.CertNo,
				CertExpiresAt: *f.CertExpiresAt,
				DaysExpired:   int(today.Time.Sub(f.CertExpiresAt.Time).Hours() / 24),
			})
		}
	}
	sort.Slice(issues.Expired, func(a, b int) bool {
		if issues.Expired[a].CertExpiresAt.Time.Equal(issues.Expired[b].CertExpiresAt.Time) {
			return issues.Expired[a].ID < issues.Expired[b].ID
		}
		return issues.Expired[a].CertExpiresAt.Time.Before(issues.Expired[b].CertExpiresAt.Time)
	})
	return issues, nil
}
