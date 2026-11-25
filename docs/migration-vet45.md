# Chuyển vet45 từ `vet45_env` sang kith

Kế hoạch cắt chuyển cho vet45, khách hàng đầu tiên của kith. Các tính năng chung mà kế hoạch này dựa vào (`secrets upload`, `use --all`, Mount, Secret file) nằm ở [PRD](./PRD.md). Nguồn quyết định: [Kế hoạch chuyển từ vet45_env sang kith](https://github.com/nd2204/kith/issues/10), cùng phần hiện trạng vet45 ở [CI và deploy nhận secret thế nào](https://github.com/nd2204/kith/issues/18) và [Secret dạng file ghi ra đường dẫn cố định](https://github.com/nd2204/kith/issues/26).

## 1. Hiện trạng

- Secret nằm ở `.vet45/profiles/{default,test,testapi}/<env>.env`, chia sẻ giữa các worktree bằng symlink.
- Có 7 `vet45.target.json` (admin, erp, api, worker, deploy, railway, ví dụ của design system), dùng `from`, `default`, `fromManifest`, `autoGenerate`, `required` và các action có template `{{…}}`.
- Environment `local` lấy giá trị động từ `supabase status`.
- CI là CircleCI, secret nằm trong context `vet45-staging`, `vet45-production`, `vet45-deploy`; job ghi `.env` từ context. Railway giữ env trong kho variables của nó. iOS build trên CircleCI macOS executor. Không đích nào nằm trong tailnet.
- Chưa có secret dạng file nào cần chuyển. Keystore Android và `GoogleService-Info.plist` đã gitignore nhưng không có trên đĩa. File `.p8` của App Store Connect do `altool` tự tìm ở `~/.appstoreconnect/private_keys/` và đang được quản lý bằng tay.
- Chỉ một người dùng vet45.

## 2. Ánh xạ Project

Theo [ADR 0004](./adr/0004-moi-app-mot-project-setup-khong-bien-doi-key.md):

| Project | Nội dung | `kith.yaml` |
|---|---|---|
| `vet45` | Project chung, giữ các key canonical lấy y nguyên từ profile. | Gốc repo: các lệnh toàn repo (`supabase:link`, admin, Railway với `dir: .railway`). |
| `vet45-erp`, `vet45-admin`, `vet45-api`, `vet45-worker` | Mỗi Project giữ đúng tập key mà target của nó liệt kê, phần lớn là tham chiếu `${vet45.<cfg>.KEY}`. | Thư mục của từng app. |
| `vet45-deploy` | Key App Store Connect. Config `production`; `staging` tham chiếu `production`. | `deploy/kith.yaml`. |

- Machine chạy một app cần Grant cả Project app lẫn `vet45`.
- Target `railway` không có Project riêng. Ví dụ của design system bị bỏ.

## 3. Profile và Environment

- Chỉ import profile `default`. `test` và `testapi` không mang sang; kith không có khái niệm profile. Cần backend thứ hai thì tự viết thành Branch config.
- Giữ nguyên tên `development`, `staging`, `production`, để các script đang nhận `--env staging` đọc thẳng `$KITH_ENVIRONMENT`.
- `local` thành Branch config tĩnh `development_local` của `development` trong mọi Project. Giá trị chụp một lần từ `supabase status`; kith không có nguồn động.

## 4. Lệnh import

- `kith secrets upload <file.env|json>`: lệnh chung (xem PRD mục 8).
- `kith import vet45 [--dry-run]`: lớp mỏng chạy một lần, dựng trên `secrets upload`.
  - Ghi Config và **bản nháp** `kith.yaml` cạnh mỗi target, rồi in báo cáo các trường không dịch được.
  - Không ghi đè `kith.yaml` đã có; từ chối chạy nếu Project đã tồn tại; không đụng repo vet45 ngoài việc tạo file mới.
  - Đánh dấu legacy: giữ suốt v1.x, gỡ ở v2.0.

### 4.1 Dịch key

| Nguồn | Kết quả |
|---|---|
| Key canonical trong profile | Chép y nguyên vào Config của `vet45`. |
| Key có `from: X` | `NAME: ${vet45.<cfg>.X}` |
| Key không có `from`, canonical có | `${vet45.<cfg>.NAME}` |
| Key không có `from`, canonical không có, có `default` | Literal trong root Config của app. |
| Còn lại | Bỏ. |
| `fromManifest: appEnv` / `flavor` / `dockerSupabaseUrl` / `railwayEnvironment` | Key literal theo Environment: `APP_ENV`, `FLAVOR`, `DOCKER_SUPABASE_URL`, `RAILWAY_ENVIRONMENT`. |
| `required: true` | `requires:` |
| `description` của key | Comment YAML trên từng mục. |
| `autoGenerate` | Sinh một lần lúc import nếu giá trị trống; sau đó không sinh tự động. |
| `envFile`, `writeEnvFile`, `secretsFile` | Bỏ. |

Branch config `development_local` của mỗi app chỉ ghi đè các tham chiếu để trỏ sang `vet45.development_local`.

### 4.2 Dịch action thành Named command

| Nguồn | Kết quả |
|---|---|
| `command` + `args` | Một dòng shell đã escape. |
| `{{flavor}}` | `"$FLAVOR"` |
| `{{environmentId}}` | `"$KITH_ENVIRONMENT"` |
| `{{railwayEnvironment}}` | `"$RAILWAY_ENVIRONMENT"` |
| `{{dockerSupabaseUrl}}` | `"$DOCKER_SUPABASE_URL"` |
| `{{envBasename}}` (Flutter) | `"$KITH_MOUNT"` kèm `mount: json` |
| `{{envFile}}` (docker compose) | `"$KITH_MOUNT"` kèm `mount: env` |
| `cwd: {{workspaceRoot}}` | `dir:` tương đối tới gốc git |
| `env: {A: …}` | Tiền tố gán biến `A="…" ./script.sh` |
| `requireEnvironments` | `environments` |
| `platforms` | Bỏ; thêm "(macOS)" vào `description`. |
| `passEnvFile`, `requireEnvFile` | Bỏ (env luôn được inject). |
| `preActions`, `requireDoctorTargets`, `requireRailwayEnvironment`, `localSupabaseCheck` | Bỏ; liệt kê trong báo cáo để người duyệt xử lý tay. |

### 4.3 iOS

- `build-ios` ở lại `kith.yaml` của app.
- `deploy-ios` chuyển sang `deploy/kith.yaml` (Project `vet45-deploy`) thành `deploy-ios:erp` và `deploy-ios:admin` với `dir: ../apps/<app>`.
- Lý do: Mount đưa cả Config vào `dart-define`, nên key App Store không được nằm trong Config của app Flutter (PRD mục 7.5).
- Không còn `preActions`: người chạy `build-ios` rồi `deploy-ios:<app>`.
- File `.p8` là ứng viên đầu tiên cho Secret file với đường dẫn `~/.appstoreconnect/private_keys/AuthKey_<id>.p8` trong `deploy-ios:<app>`.
- Giả định `deploy-ios.sh` chỉ cần `APP_STORE_CONNECT_KEY_IDENTIFIER` và `ISSUER_ID`. Người duyệt bản nháp kiểm tra lại.

## 5. Thay thế các lệnh cũ

| `vet45_env` | Thay bằng |
|---|---|
| `use`/`switch`/`use-and-sync`/`use-sync-run` | `kith use`, `kith use --all` |
| `sync` | Không cần: env được inject lúc `run`, `autoGenerate` đã làm lúc import. |
| `resolve`/`resolve-shell` | `kith secrets download --format env`, `kith run -- $SHELL` |
| `link`, delegate `supabase` | Named command `supabase:link` trong `kith.yaml` gốc. |
| `auth token` (`SUPABASE_ACCESS_TOKEN`) | Không import. Dùng `supabase login`. |
| `admin create\|grant` | Script Dart riêng trong vet45 (`tools/platform_admin`), gọi qua Named command ở gốc. |
| `doctor` (`alignHosts`) | Không cần: tham chiếu bảo đảm nhất quán. Bỏ kiểm tra công cụ. |
| Bộ sinh `.env.example` | Bỏ: `requires:` là tài liệu. |
| `.railway` `plan/apply/sync-env/rollout` | Named command trong `kith.yaml` gốc. `sync-env` theo recipe xuất một chiều (PRD mục 16). |
| `completion` | `kith completion`. |

CircleCI: làm mới các context `vet45-staging`, `vet45-production`, `vet45-deploy` bằng Named command nối `kith secrets download` vào `circleci context store-secret`.

## 6. Cùng tồn tại

- Chuyển theo từng Config. Từ lúc một Config được import, kith là nguồn sự thật.
- App nào chưa chuyển mà vẫn cần `vet45_env` thì sinh lại file profile bằng `kith secrets download --out .vet45/profiles/default/<env>.env`. Luồng một chiều, chạy tay, không sửa file cũ.

## 7. Thứ tự cắt chuyển

1. Trên máy chính: `kith import vet45`, duyệt bản nháp rồi commit `kith.yaml` trên một nhánh. Chuyển `development` và `development_local` cho `api` và `worker`.
2. `erp` và `admin`, gồm Mount và recipe wrapper `launch.json`. Xóa `.env` trong thư mục app khi app đó chuyển xong.
3. Các Machine khác của chính mình `join` qua Invite.
4. `staging` rồi `production`: `build-ios`/`deploy-ios`, xuất sang Railway, làm mới context CircleCI.

## 8. Tiêu chí gỡ tool cũ

Theo điều kiện, không theo ngày:

- [ ] Mọi action có Named command chạy được, hoặc nằm trong danh sách bỏ.
- [ ] Mọi Machine đã join.
- [ ] Đã làm trọn một lần release staging và production bằng kith.
- [ ] Không thư mục app nào còn `.env` do `vet45_env` ghi.

Sau đó một PR trên vet45 xóa `tools/vet45_env`, mọi `vet45.target.json`, `vet45.workspace.json` và các link `.vet45` trong worktree. `agapi/vet45/.vet45/profiles` bị xóa cuối cùng, sau khi đã có bản sao ở Recovery machine hoặc Machine thứ hai.
