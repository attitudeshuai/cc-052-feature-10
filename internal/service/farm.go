package service

import (
	"cc-052/internal/model"
	"cc-052/internal/normalize"
	"cc-052/internal/repository"
	"fmt"
	"sort"
	"strings"
	"time"
)

type FarmService struct {
	repo *repository.FarmRepo
}

func NewFarmService(repo *repository.FarmRepo) *FarmService {
	return &FarmService{repo: repo}
}

// ValidationError marks bad client input (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// NotFoundError marks a missing farm (HTTP 404).
type NotFoundError struct{ ID int64 }

func (e *NotFoundError) Error() string { return fmt.Sprintf("farm %d not found", e.ID) }

// ConflictError marks a normalized (region, name) duplicate (HTTP 409) and
// carries the existing archive so the caller can see exactly who is there.
type ConflictError struct{ Existing *model.Farm }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("该地区已存在同名档案：%s（ID %d）", e.Existing.Name, e.Existing.ID)
}

func (s *FarmService) Create(req *model.CreateFarmRequest) (*model.Farm, error) {
	name := strings.TrimSpace(req.Name)
	region := strings.TrimSpace(req.RegionCode)
	certNo := strings.TrimSpace(req.CertNo)
	if name == "" || region == "" {
		return nil, &ValidationError{Msg: "名称与地区编码不能为空"}
	}
	if certNo == "" {
		return nil, &ValidationError{Msg: "资质编号不能为空"}
	}
	expiresAt, err := parseCertExpiry(req.CertExpiresAt)
	if err != nil {
		return nil, err
	}

	farm := &model.Farm{
		Name:          name,
		RegionCode:    region,
		ContactRef:    strings.TrimSpace(req.ContactRef),
		CertNo:        certNo,
		CertExpiresAt: expiresAt,
		NameNorm:      normalize.Name(name),
		RegionNorm:    normalize.Region(region),
	}

	conflict, err := s.findConflict(farm.RegionNorm, farm.NameNorm, 0)
	if err != nil {
		return nil, err
	}
	if conflict != nil {
		return nil, &ConflictError{Existing: conflict}
	}

	if err := s.repo.Create(farm); err != nil {
		return nil, err
	}
	return farm, nil
}

func (s *FarmService) GetByID(id int64) (*model.Farm, error) {
	return s.repo.GetByID(id)
}

func (s *FarmService) List() ([]model.Farm, error) {
	return s.repo.List()
}

// Update applies a partial edit. Name/region changes are re-normalized,
// re-checked for duplicates and recorded with their before/after values.
func (s *FarmService) Update(id int64, req *model.UpdateFarmRequest) (*model.Farm, error) {
	farm, err := s.repo.GetByID(id)
	if err != nil {
		return nil, &NotFoundError{ID: id}
	}

	var logs []model.FarmChangeLog

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, &ValidationError{Msg: "名称不能为空"}
		}
		if name != farm.Name {
			logs = append(logs, model.FarmChangeLog{FarmID: id, Field: "name", OldValue: farm.Name, NewValue: name})
			farm.Name = name
		}
	}
	if req.RegionCode != nil {
		region := strings.TrimSpace(*req.RegionCode)
		if region == "" {
			return nil, &ValidationError{Msg: "地区编码不能为空"}
		}
		if region != farm.RegionCode {
			logs = append(logs, model.FarmChangeLog{FarmID: id, Field: "region_code", OldValue: farm.RegionCode, NewValue: region})
			farm.RegionCode = region
		}
	}
	if req.ContactRef != nil {
		farm.ContactRef = strings.TrimSpace(*req.ContactRef)
	}
	if req.CertNo != nil {
		certNo := strings.TrimSpace(*req.CertNo)
		if certNo == "" {
			return nil, &ValidationError{Msg: "资质编号不能为空"}
		}
		farm.CertNo = certNo
	}
	if req.CertExpiresAt != nil {
		expiresAt, err := parseCertExpiry(*req.CertExpiresAt)
		if err != nil {
			return nil, err
		}
		farm.CertExpiresAt = expiresAt
	}

	farm.NameNorm = normalize.Name(farm.Name)
	farm.RegionNorm = normalize.Region(farm.RegionCode)

	if len(logs) > 0 {
		conflict, err := s.findConflict(farm.RegionNorm, farm.NameNorm, id)
		if err != nil {
			return nil, err
		}
		if conflict != nil {
			return nil, &ConflictError{Existing: conflict}
		}
	}

	if err := s.repo.UpdateWithLogs(farm, logs); err != nil {
		return nil, err
	}
	return farm, nil
}

// Changes returns the name/region change history of one farm.
func (s *FarmService) Changes(farmID int64) ([]model.FarmChangeLog, error) {
	if _, err := s.repo.GetByID(farmID); err != nil {
		return nil, &NotFoundError{ID: farmID}
	}
	return s.repo.ListChanges(farmID)
}

// CertIssues lists duplicate qualification numbers and expired
// qualifications (with their expiry dates) as two separate lists.
func (s *FarmService) CertIssues() (*model.CertIssuesReport, error) {
	farms, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	report := &model.CertIssuesReport{
		Duplicates: []model.DuplicateCertGroup{},
		Expired:    []model.CertIssueFarm{},
	}

	groups := map[string][]model.Farm{}
	for _, f := range farms {
		key := normalize.CertNo(f.CertNo)
		if key == "" {
			continue // 历史空资质不参与判重
		}
		groups[key] = append(groups[key], f)
	}

	keys := make([]string, 0, len(groups))
	for key, members := range groups {
		if len(members) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		members := groups[key]
		group := model.DuplicateCertGroup{CertNo: key, Count: len(members)}
		for _, f := range members {
			group.Farms = append(group.Farms, certIssueFarm(f))
		}
		report.Duplicates = append(report.Duplicates, group)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, f := range farms {
		if f.CertExpiresAt == nil {
			continue
		}
		if expires := f.CertExpiresAt.UTC().Truncate(24 * time.Hour); expires.Before(today) {
			report.Expired = append(report.Expired, certIssueFarm(f))
		}
	}
	return report, nil
}

// NormalizeField rewrites every legacy spelling listed in aliases to the
// canonical value in one transaction. The result carries the farm row count
// before and after so callers can verify no record was lost.
func (s *FarmService) NormalizeField(req *model.NormalizeFieldRequest) (*model.NormalizeFieldResult, error) {
	var normFunc func(string) string
	switch req.Field {
	case "region_code":
		normFunc = normalize.Region
	case "name":
		normFunc = normalize.Name
	default:
		return nil, &ValidationError{Msg: "field 仅支持 region_code 或 name"}
	}

	canonical := strings.TrimSpace(req.Canonical)
	if canonical == "" {
		return nil, &ValidationError{Msg: "canonical 不能为空"}
	}

	aliasSet := map[string]bool{}
	for _, a := range req.Aliases {
		if key := normFunc(a); key != "" {
			aliasSet[key] = true
		}
	}
	if len(aliasSet) == 0 {
		return nil, &ValidationError{Msg: "aliases 不能为空"}
	}

	farms, err := s.repo.List()
	if err != nil {
		return nil, err
	}

	var changes []repository.FieldChange
	for _, f := range farms {
		current := f.RegionCode
		if req.Field == "name" {
			current = f.Name
		}
		if current == canonical {
			continue // 已是目标取值
		}
		if aliasSet[normFunc(current)] {
			changes = append(changes, repository.FieldChange{FarmID: f.ID, OldValue: current})
		}
	}

	before, after, err := s.repo.ApplyNormalize(req.Field, canonical, normFunc(canonical), changes)
	if err != nil {
		return nil, err
	}

	return &model.NormalizeFieldResult{
		Field:       req.Field,
		Canonical:   canonical,
		Updated:     len(changes),
		TotalBefore: before,
		TotalAfter:  after,
		CountsMatch: before == after,
	}, nil
}

// BackfillNorms recomputes normalized keys for legacy rows at startup.
func (s *FarmService) BackfillNorms() (int, error) {
	farms, err := s.repo.List()
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, f := range farms {
		nameNorm := normalize.Name(f.Name)
		regionNorm := normalize.Region(f.RegionCode)
		if f.NameNorm != nameNorm || f.RegionNorm != regionNorm {
			if err := s.repo.UpdateNorms(f.ID, nameNorm, regionNorm); err != nil {
				return updated, err
			}
			updated++
		}
	}
	return updated, nil
}

// findConflict returns the oldest existing farm holding the same normalized
// (region, name) key, or nil. excludeID skips the farm being edited.
func (s *FarmService) findConflict(regionNorm, nameNorm string, excludeID int64) (*model.Farm, error) {
	farms, err := s.repo.FindByNorm(regionNorm, nameNorm, excludeID)
	if err != nil {
		return nil, err
	}
	if len(farms) == 0 {
		return nil, nil
	}
	return &farms[0], nil
}

func parseCertExpiry(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, &ValidationError{Msg: "资质到期日不能为空（格式 YYYY-MM-DD）"}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, &ValidationError{Msg: "资质到期日格式应为 YYYY-MM-DD"}
	}
	return &t, nil
}

func certIssueFarm(f model.Farm) model.CertIssueFarm {
	item := model.CertIssueFarm{
		FarmID:     f.ID,
		Name:       f.Name,
		RegionCode: f.RegionCode,
		CertNo:     f.CertNo,
	}
	if f.CertExpiresAt != nil {
		item.CertExpiresAt = f.CertExpiresAt.Format("2006-01-02")
	}
	return item
}
