# 农村合作社农产品溯源服务 · Farm Traceability Service

> 类型：后端 API 服务｜难度：★★★｜技术栈：**Go + Gin + PostgreSQL + MinIO（图片）+ Redis**（统一选型）

## 1. 一句话简介
给合作社每一批农产品发一个溯源码，从「谁家几号地块、哪天施肥打药、哪天采收、谁检测」一路查到消费者扫码看到的页面数据。

## 2. 真实场景与痛点
- 农产品想卖进平台/商超，第一关就是「有没有溯源」；合作社拿 Excel 记账，一查批次就断链。
- 消费者扫二维码看到的多是营销页，没有真数据。
- 十几个农户共用一台设备轮流上报，离线是常态，网络恢复后还要合并数据。

## 3. 目标用户
- 村级合作社、家庭农场、农产品区域公用品牌运营方。
- 想给自家水果贴溯源码的种植大户。

## 4. 核心功能（MVP）
1. **生产档案**：地块（地理坐标/面积/土壤）、作物、种植批次（`batch_id`）。
2. **农事记录上报**：施肥/用药/灌溉/除草，含时间、投入品名称、用量、操作人、照片；支持**离线批量补报**（客户端带 `client_uuid`，服务端幂等去重）。
3. **采收与检测**：采收日期、产量、农残检测报告（图片 + 结论），检测不合格批次直接锁定不可发码。
4. **溯源码生成**：一个批次拆成若干包装单位，批量生成唯一 `trace_code`（短码 + 校验位，防手输错误）。
5. **扫码查询**：公开只读接口，输入 code 返回溯源链（脱敏：不暴露农户手机号/精确坐标，只给村级位置）。
6. **基础资料**：投入品字典（农药登记证号、安全间隔期）。

## 5. 进阶功能
- 安全间隔期校验：距上次施药不足间隔期就采收 → 返回 `WARNING` 并禁止发码。
- 二维码预生成 PDF 印刷文件（批量、含排版）。
- 防伪：一个码首次被扫记录首次扫码时间与地区，二次扫码提示。
- 数据导出给监管平台（JSON/XML 标准格式适配器）。

## 6. 接口设计（节选）
```
POST /api/v1/farms                          创建农场/合作社（归一化判重，见第 13 节）
PUT  /api/v1/farms/{id}                     修改档案（名称/地区/资质改动自动留前后两版）
GET  /api/v1/farms/{id}/revisions           档案改动历史
GET  /api/v1/reports/cert-issues            资质编号重复 + 已到期清单（给到期日）
POST /api/v1/farms/cleanup                  老数据归一化清洗（默认 dry_run，body {"apply":true} 落库）
GET  /api/v1/regions                        行政区划字典（?level=3 过滤县级）
POST /api/v1/regions/aliases                登记地区老写法别名（{"alias":"青龙县","region_code":"130321"}）
POST /api/v1/plots                          地块登记
POST /api/v1/batches                        创建种植批次
POST /api/v1/batches/{id}/activities        农事记录（支持数组批量，client_uuid 幂等）
POST /api/v1/batches/{id}/inspection        上传检测结果
POST /api/v1/batches/{id}/codes             生成溯源码（返回数量与短码列表）
GET  /api/v1/trace/{code}                   公开溯源查询（无需鉴权，限流）
GET  /api/v1/trace/{code}/qrcode            返回二维码 PNG（带缓存头）
```

## 13. 合作社档案：归一化判重 / 资质核验 / 历史留痕 / 老数据清洗

痛点：地区编码与资质编号手写随心、同县多种写法、资质空着也建档、名字差一个字分不清。

- **建档判重（`POST /farms`）**：服务端对名称与地区做归一化（NFKC 全半角折叠、大小写、去空白、剥「农民专业合作社/专业合作社/合作社/家庭农场/公司」后缀；地区支持 6 位/12 位码、别名、标准名、核心名兜底）。按 `(归一地区, 归一名称)` 判重，命中返回 **409** 并在 `data.existing` 中给出已存在的是哪一家；资质编号归一后被别家占用同样 409 并指出占用方。**空资质允许建档，且两家都空不互相冲突**；资质已到期不阻断建档，在 `warnings` 中给出到期日与过期天数。
- **资质问题清单（`GET /reports/cert-issues`）**：`duplicates` 按归一组列出同号的所有合作社（含各家原始写法 `raw_cert_nos`），`expired` 给出到期日与过期天数。
- **改动留痕（`PUT /farms/{id}`）**：名称、地区、资质编号发生变化时，在 `farm_revision` 中保存改动前后两版，`GET /farms/{id}/revisions` 可查；修改后撞名/撞资质号同样 409。
- **老数据清洗（`POST /farms/cleanup`）**：默认 `dry_run` 只出方案；`{"apply":true}` 单事务逐行把地区写法改到标准码、组内名称统一、资质号大写归一，并为每行写 revision。返回条数对账（`total_scanned == total_after`、`changed+unchanged+unresolved == total`），**清洗不删行、不自动合并**：归一后仍重名的在 `collisions` 列出（如涉及地块/批次归属需人工裁定），无法识别的地区在 `unresolved` 列出，登记别名后可再次清洗。二次清洗幂等。
- **字典与别名**：`region_dict`（标准码）+ `region_alias`（老写法→标准码），种子见 `seed/regions_and_farms.sql`，迁移见 `migrations/002_farm_normalization.sql`。

## 7. 数据模型
```sql
farm(id, name, region_code, contact_ref, cert_no)
plot(id, farm_id, name, area_mu, geojson /* 简化多边形 */, soil_type)
crop_batch(id, plot_id, crop_id, sowing_date, harvest_date, expected_yield_kg, status /* growing|harvested|locked */)
activity(id, batch_id, client_uuid UNIQUE, kind /* fertilize|pesticide|irrigation|weed */, happened_at,
         input_id, dose, dose_unit, operator, photos jsonb, geo, created_at)
input_material(id, name, type, registration_no, safe_interval_days, active_ingredient)
inspection(id, batch_id, lab, sampled_at, result /* pass|fail */, report_url, items jsonb)
trace_code(id, batch_id, code UNIQUE, seq, printed_at, first_scanned_at, first_scan_region)
```

## 8. 关键实现点
- **幂等补报**：`activity.client_uuid` 唯一索引 + `ON CONFLICT DO NOTHING`，离线重传不会产生重复记录。
- **短码设计**：`Base32(时间戳低 32 位 + 批次序号 + CRC8)` 共 10 位，带校验位，扫码和手输都可。
- **时序完整性**：写入 activity 时校验 `happened_at` 不早于播种、不晚于采收，非法则拒绝（或标 `needs_review`）。
- **图片处理**：上传走预签名 URL 直传 MinIO，服务端只存 key；生成缩略图用于扫码页。
- **公开接口防护**：`/trace/{code}` 按 IP 限流（Redis 令牌桶，如 30 次/分钟），并对返回体脱敏。
- **安全间隔期**：发码时 `SELECT max(happened_at)` 与 `harvest_date` 比较，不足则拒绝。

## 9. 技术约束与性能
- 所有时间存 UTC，展示按 `region_code` 转换（农事日期以当地日期为准，避免跨零点算错一天）。
- 溯源查询必须扛住扫码高峰（如直播带货瞬间），QPS 目标 500，走 Redis 缓存 5 分钟 + 缓存穿透保护。
- 码生成 10 万条用 `COPY`/批量 insert，单批 1000 条，避免长事务。

## 10. 验收标准
- 离线 200 条农事记录恢复网络后重传 3 次，数据库记录数仍为 200。
- 单次生成 10 万溯源码 < 30s，无重复（唯一约束兜底）。
- 检测失败或间隔期不足的批次 100% 无法发码。
- 扫码接口 P99 < 80ms，缓存命中率 > 95%。

## 11. 边界（刻意不做）
不做农产品在线交易/订单/结算，不做库存 ERP，不做物流跟踪——避开黑名单中的电商订单、仓库库存类系统。

## 12. 容器化与构建（Docker）

本项目交付**必须能通过 Docker 构建与运行**，验收以 `docker compose up` 后对容器发起真实 HTTP 请求的结果为准。

- **Dockerfile（多阶段，统一 Go 模板）**
  - `builder`：`golang:1.22-alpine`，先 `COPY go.mod go.sum` 再 `go mod download`，然后 `CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/app ./cmd/api`
  - `runtime`：`gcr.io/distroless/static-debian12:nonroot`，只放单个静态二进制（自带 ca-certificates 与 tzdata），**非 root**、无 shell
  - BuildKit cache mount（`/go/pkg/mod`、`/root/.cache/go-build`）→ 只改业务代码时增量构建 5~15s
  - 健康检查：distroless 无 shell，不能写 `curl`，用 `HEALTHCHECK CMD ["/app","health"]`
- **docker-compose.yml**（服务名 `cc-052`）
  - `api`：端口 `9052:8080`，`restart: unless-stopped`
  - `db`：`postgres:16-alpine`（如用 jsonb 存农事照片元数据足够），数据卷 `pgdata` 持久化
  - `cache`：`redis:7-alpine`（扫码查询缓存 + 令牌桶限流）
  - `minio`：`minio/minio`，用于农事照片与检测报告，数据卷 `miniodata` 持久化
- **对象存储易踩的坑**
  - 预签名 URL 里的 host 必须是**客户端能访问到的地址**，容器内是 `minio:9000`、容器外是 `localhost:9000` → 用独立配置项 `MINIO_PUBLIC_ENDPOINT`，不能用容器内地址拼 URL
  - 启动时自动创建 bucket（幂等），并设置最小必要权限
- **数据库迁移**：入口脚本 `migrate && start`，幂等；农事记录表建 `client_uuid` 唯一索引（离线补报幂等的关键）
- **定时任务单实例**：间隔期校验与批量发码的定时任务**只允许一个实例执行**（Redis 分布式锁或 `replicas: 1`），否则会重复发通知
- **健康检查**：`HEALTHCHECK` → `GET /healthz`（含 db / cache / minio 状态）
- **日志与配置**：stdout 结构化日志；密钥走 `.env` + `secrets`，禁止硬编码 DB / MinIO 凭据

```bash
cd cc-052/cc-052
cp .env.example .env            # 填 DB / Redis / MinIO 与对外访问地址
docker compose up -d --build     # 起 api + db + cache + minio
curl http://localhost:9052/healthz
docker compose logs -f api
docker compose down              # 加 -v 一并清数据卷
```

- **验收**：容器内跑通「创建批次 → 离线补报 200 条并重传 3 次 → 生成 10 万溯源码 → 扫码查询」全链路，记录数与第 10 节一致；照片在容器外浏览器可直接显示（验证预签名 URL 地址正确）；重启后数据与文件仍在。

### 忽略文件（.gitignore / .dockerignore）

交付时必须**同时**提供 `.dockerignore` 与 `.gitignore`，两者作用不同、缺一不可（`.gitignore` 对 `docker build` 无效，反之亦然）。

- **`.dockerignore`**（决定构建上下文）
  ```
  bin
  tmp
  .cache
  coverage.out
  .git
  .gitignore
  .env
  .env.*
  *.log
  coverage
  .vscode
  .idea
  Dockerfile
  docker-compose.yml
  README.md
  ```
  - **不要**忽略 `go.mod` / `go.sum` 与 `migrations/`、`*.sql`（依赖解析与入口 `migrate` 都需要）
  - 投入品字典等基础数据脚本（`seed/*`）必须保留
  - **保留** `.env.example`，只忽略真实 `.env`
  - 本地测试用的模拟农事照片（`testdata/photos/`）建议忽略，避免把大文件写进仓库与镜像
- **`.gitignore`**
  ```
  bin/
  tmp/
  coverage.out
  *.test
  .env
  .env.local
  *.log
  coverage/
  .DS_Store
  .vscode/
  .idea/
  testdata/photos/
  *.mp4
  ```
- **安全自检**：`docker history <img>` 无密钥；`git status` 不出现 `.env` 与大体积测试素材；MinIO 凭据只在运行时注入
