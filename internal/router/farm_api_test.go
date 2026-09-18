package router_test

import (
	"bytes"
	"cc-052/internal/handler"
	"cc-052/internal/model"
	"cc-052/internal/repository"
	"cc-052/internal/router"
	"cc-052/internal/service"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// Integration test for farm archive governance. Runs against a real
// PostgreSQL when TEST_PG_DSN is set, e.g.:
//
//	TEST_PG_DSN="host=localhost port=5432 user=farm password=farm_secret dbname=farm_trace_test sslmode=disable" go test ./internal/router/
type apiResp struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func setupTestRouter(t *testing.T) (*gin.Engine, *sqlx.DB, *service.FarmService) {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set, skipping integration test")
	}
	gin.SetMode(gin.TestMode)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	if err := repository.RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	farmRepo := repository.NewFarmRepo(db)
	plotRepo := repository.NewPlotRepo(db)
	batchRepo := repository.NewBatchRepo(db)
	activityRepo := repository.NewActivityRepo(db)
	inspectionRepo := repository.NewInspectionRepo(db)
	codeRepo := repository.NewTraceCodeRepo(db)

	farmSvc := service.NewFarmService(farmRepo)
	plotSvc := service.NewPlotService(plotRepo)
	batchSvc := service.NewBatchService(batchRepo, plotRepo, farmRepo)
	activitySvc := service.NewActivityService(activityRepo, batchRepo)
	inspectionSvc := service.NewInspectionService(inspectionRepo, batchRepo)
	traceCodeSvc := service.NewTraceCodeService(codeRepo, batchRepo, inspectionRepo, activityRepo, plotRepo, farmRepo)

	r := router.Setup(
		handler.NewFarmHandler(farmSvc),
		handler.NewPlotHandler(plotSvc),
		handler.NewBatchHandler(batchSvc),
		handler.NewActivityHandler(activitySvc),
		handler.NewInspectionHandler(inspectionSvc),
		handler.NewTraceCodeHandler(traceCodeSvc),
		handler.NewHealthHandler(db, nil),
		nil,
	)
	return r, db, farmSvc
}

func doRequest(t *testing.T, r *gin.Engine, method, path, body string) (int, apiResp) {
	t.Helper()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp apiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s %s: decode response: %v (body: %s)", method, path, err, w.Body.String())
	}
	return w.Code, resp
}

func decodeData[T any](t *testing.T, resp apiResp) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(resp.Data, &v); err != nil {
		t.Fatalf("decode data: %v (raw: %s)", err, string(resp.Data))
	}
	return v
}

// seedLegacyFarms inserts pre-governance rows: messy region spellings,
// duplicate/empty certs, missing expiry, empty normalized keys.
func seedLegacyFarms(t *testing.T, db *sqlx.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO farm (name, region_code, contact_ref, cert_no, cert_expires_at, name_norm, region_norm) VALUES
		('绿源蔬菜专业合作社', '寿光市', '', 'ABC-123', '2028-01-01', '', ''),
		('绿源 蔬菜专业合作社', '寿光', '', 'abc-123 ', '2020-01-01', '', ''),
		('丰收合作社', 'shouguang', '', 'XYZ-9', NULL, '', ''),
		('丰收农业合作社', 'SHOUGUANG', '', 'XYZ-9', '2027-06-01', '', ''),
		('空白资质社', '青州', '', '', NULL, '', ''),
		('空白资质二社', '青州', '', '', NULL, '', '')`)
	if err != nil {
		t.Fatalf("seed legacy farms: %v", err)
	}
}

func TestFarmArchiveGovernance(t *testing.T) {
	r, db, farmSvc := setupTestRouter(t)
	defer db.Close()
	seedLegacyFarms(t, db)

	// 1. 启动回填：老数据的归一键被补齐
	t.Run("backfill norms", func(t *testing.T) {
		n, err := farmSvc.BackfillNorms()
		if err != nil {
			t.Fatalf("backfill: %v", err)
		}
		if n != 6 {
			t.Fatalf("backfill updated %d rows, want 6", n)
		}
		var missing int
		if err := db.Get(&missing, `SELECT COUNT(*) FROM farm WHERE name_norm = '' OR region_norm = ''`); err != nil {
			t.Fatal(err)
		}
		if missing != 0 {
			t.Fatalf("%d rows still missing norms", missing)
		}
	})

	// 2. 建档判重：归一后地区+名称相同 → 409 并指出已存在的是哪一家
	t.Run("create duplicate conflict", func(t *testing.T) {
		status, resp := doRequest(t, r, "POST", "/api/v1/farms",
			`{"name":"绿源蔬菜专业合作社","region_code":"寿光县","cert_no":"NEW-1","cert_expires_at":"2030-01-01"}`)
		if status != http.StatusConflict || resp.Code != http.StatusConflict {
			t.Fatalf("status=%d code=%d, want 409 (msg=%s)", status, resp.Code, resp.Message)
		}
		existing := decodeData[model.Farm](t, resp)
		if existing.ID != 1 || existing.Name != "绿源蔬菜专业合作社" {
			t.Fatalf("conflict should point at farm 1, got %+v", existing)
		}
	})

	// 3. 正常建档
	t.Run("create success", func(t *testing.T) {
		status, resp := doRequest(t, r, "POST", "/api/v1/farms",
			`{"name":"新绿合作社","region_code":"昌乐县","cert_no":"NEW-2","cert_expires_at":"2030-06-01"}`)
		if status != http.StatusCreated {
			t.Fatalf("status=%d, want 201 (msg=%s)", status, resp.Message)
		}
		created := decodeData[model.Farm](t, resp)
		if created.ID != 7 || created.CertExpiresAt == nil {
			t.Fatalf("unexpected created farm: %+v", created)
		}
	})

	// 4. 资质空着不能建档
	t.Run("create requires cert", func(t *testing.T) {
		status, _ := doRequest(t, r, "POST", "/api/v1/farms",
			`{"name":"缺资质合作社","region_code":"青州"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400", status)
		}
	})

	// 5. 到期日格式校验
	t.Run("create validates expiry format", func(t *testing.T) {
		status, _ := doRequest(t, r, "POST", "/api/v1/farms",
			`{"name":"甲合作社","region_code":"昌乐","cert_no":"C-1","cert_expires_at":"2027/01/01"}`)
		if status != http.StatusBadRequest {
			t.Fatalf("status=%d, want 400", status)
		}
	})

	// 6. 资质问题清单：重复编号与已到期分开列出，到期给出到期日
	t.Run("cert issues report", func(t *testing.T) {
		status, resp := doRequest(t, r, "GET", "/api/v1/farms/cert-issues", "")
		if status != http.StatusOK {
			t.Fatalf("status=%d, want 200", status)
		}
		report := decodeData[model.CertIssuesReport](t, resp)

		dupCerts := map[string]int{}
		for _, g := range report.Duplicates {
			dupCerts[g.CertNo] = g.Count
		}
		if dupCerts["ABC-123"] != 2 || dupCerts["XYZ-9"] != 2 {
			t.Fatalf("duplicates = %v, want ABC-123:2 and XYZ-9:2", dupCerts)
		}
		if len(report.Duplicates) != 2 {
			t.Fatalf("empty certs must not be grouped, got %d groups", len(report.Duplicates))
		}

		if len(report.Expired) != 1 {
			t.Fatalf("expired = %+v, want exactly 1 entry", report.Expired)
		}
		exp := report.Expired[0]
		if exp.FarmID != 2 || exp.CertExpiresAt != "2020-01-01" {
			t.Fatalf("expired entry = %+v, want farm 2 with expiry 2020-01-01", exp)
		}
	})

	// 7. 改名/改地区留痕：前后两版都查得到
	t.Run("update keeps change history", func(t *testing.T) {
		status, resp := doRequest(t, r, "PUT", "/api/v1/farms/3",
			`{"name":"丰收专业合作社","region_code":"寿光市"}`)
		if status != http.StatusOK {
			t.Fatalf("status=%d (msg=%s)", status, resp.Message)
		}

		status, resp = doRequest(t, r, "GET", "/api/v1/farms/3/changes", "")
		if status != http.StatusOK {
			t.Fatalf("changes status=%d", status)
		}
		logs := decodeData[[]model.FarmChangeLog](t, resp)
		if len(logs) != 2 {
			t.Fatalf("got %d change logs, want 2: %+v", len(logs), logs)
		}
		byField := map[string]model.FarmChangeLog{}
		for _, l := range logs {
			byField[l.Field] = l
		}
		if l := byField["name"]; l.OldValue != "丰收合作社" || l.NewValue != "丰收专业合作社" {
			t.Fatalf("name log = %+v", l)
		}
		if l := byField["region_code"]; l.OldValue != "shouguang" || l.NewValue != "寿光市" {
			t.Fatalf("region log = %+v", l)
		}
	})

	// 8. 改名撞车也要被拦下
	t.Run("update rename conflict", func(t *testing.T) {
		status, _ := doRequest(t, r, "PUT", "/api/v1/farms/4",
			`{"name":"绿源蔬菜专业合作社","region_code":"寿光"}`)
		if status != http.StatusConflict {
			t.Fatalf("status=%d, want 409", status)
		}
	})

	// 9. 老数据批量归一：几种写法一次改到同一取值，前后条数对得上
	t.Run("batch normalize region", func(t *testing.T) {
		status, resp := doRequest(t, r, "GET", "/api/v1/farms", "")
		farmsBefore := decodeData[[]model.Farm](t, resp)
		if status != http.StatusOK || len(farmsBefore) != 7 {
			t.Fatalf("before: status=%d farms=%d", status, len(farmsBefore))
		}

		status, resp = doRequest(t, r, "POST", "/api/v1/farms/normalize",
			`{"field":"region_code","canonical":"370783","aliases":["寿光市","寿光","shouguang"]}`)
		if status != http.StatusOK {
			t.Fatalf("normalize status=%d (msg=%s)", status, resp.Message)
		}
		result := decodeData[model.NormalizeFieldResult](t, resp)
		if result.Updated != 4 {
			t.Fatalf("updated=%d, want 4", result.Updated)
		}
		if !result.CountsMatch || result.TotalBefore != 7 || result.TotalAfter != 7 {
			t.Fatalf("counts = %d -> %d (match=%v)", result.TotalBefore, result.TotalAfter, result.CountsMatch)
		}

		// 改完条数不变、取值统一
		_, resp = doRequest(t, r, "GET", "/api/v1/farms", "")
		farmsAfter := decodeData[[]model.Farm](t, resp)
		if len(farmsAfter) != len(farmsBefore) {
			t.Fatalf("farm count changed: %d -> %d", len(farmsBefore), len(farmsAfter))
		}
		for _, f := range farmsAfter {
			if f.ID <= 4 && f.RegionCode != "370783" {
				t.Fatalf("farm %d region = %q, want 370783", f.ID, f.RegionCode)
			}
		}

		// 批量归一同样留痕
		_, resp = doRequest(t, r, "GET", "/api/v1/farms/1/changes", "")
		logs := decodeData[[]model.FarmChangeLog](t, resp)
		if len(logs) != 1 || logs[0].OldValue != "寿光市" || logs[0].NewValue != "370783" {
			t.Fatalf("normalize change log = %+v", logs)
		}
	})

	// 10. 名称写法同样能一次归一
	t.Run("batch normalize name", func(t *testing.T) {
		status, resp := doRequest(t, r, "POST", "/api/v1/farms/normalize",
			`{"field":"name","canonical":"绿源蔬菜专业合作社","aliases":["绿源 蔬菜专业合作社"]}`)
		if status != http.StatusOK {
			t.Fatalf("status=%d (msg=%s)", status, resp.Message)
		}
		result := decodeData[model.NormalizeFieldResult](t, resp)
		if result.Updated != 1 || !result.CountsMatch {
			t.Fatalf("result = %+v, want updated=1 counts match", result)
		}
	})

	// 11. 不存在的档案
	t.Run("not found", func(t *testing.T) {
		status, _ := doRequest(t, r, "PUT", "/api/v1/farms/999", `{"name":"x"}`)
		if status != http.StatusNotFound {
			t.Fatalf("update status=%d, want 404", status)
		}
		status, _ = doRequest(t, r, "GET", "/api/v1/farms/999/changes", "")
		if status != http.StatusNotFound {
			t.Fatalf("changes status=%d, want 404", status)
		}
	})
}
