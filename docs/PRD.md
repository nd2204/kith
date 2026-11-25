# kith v1: PRD

Spec đủ để giao đi implement kith v1. Tài liệu này gom các quyết định đã chốt trên [lộ trình kith v1](https://github.com/nd2204/kith/issues/1) thành một chỗ. Lý do của từng quyết định nằm ở ticket và ADR được dẫn ở cuối mỗi mục; khi PRD và nguồn lệch nhau thì ADR thắng, rồi tới comment kết luận của ticket, và PRD phải được sửa lại.

Thuật ngữ viết hoa (Project, Config, Machine, Change…) theo đúng nghĩa trong [GLOSSARY.md](../GLOSSARY.md). Tài liệu viết tiếng Việt; tên lệnh, cờ, thông báo và giao diện của kith là tiếng Anh.

Kế hoạch chuyển vet45 từ tool cũ nằm ở file riêng: [migration-vet45.md](./migration-vet45.md).

---

## 1. Vấn đề

Dev cần secret (URL database, API key, khóa ký app) trên nhiều máy và trong nhiều repo. Hiện có hai cách, và cả hai đều không vừa:

- **File `.env` trong thư mục**, như tool cũ `vet45_env` (Dart): secret nằm rải rác ở `.vet45/profiles/<profile>/<env>.env`, chia sẻ giữa các worktree bằng symlink, chép tay giữa các máy, không có lịch sử, không thu hồi được từng máy, và dễ lỡ commit.
- **Dịch vụ tập trung kiểu Doppler**: UX tốt (`setup`, `run -- <cmd>`, Branch config, tham chiếu), nhưng mọi secret nằm trên server của bên thứ ba, cần mạng để đọc, và tốn tiền theo user.

kith giữ UX của Doppler nhưng bỏ server: secret được nhân bản trực tiếp giữa các máy tin cậy trong một mạng riêng có sẵn (Tailscale hoặc Headscale), mã hóa end-to-end tới từng máy được cấp quyền, và luôn đọc được từ bản sao cục bộ kể cả khi offline.

## 2. Mục tiêu

1. **Không còn `.env` trong repo.** Secret được inject vào lệnh lúc chạy bằng `kith run`. Công cụ cần file thì có file tạm, sống đúng bằng vòng đời lệnh.
2. **Không có server trung tâm.** Machine nhân bản với nhau qua tailnet có sẵn. kith không tự xây mesh, không làm NAT traversal, không có relay.
3. **Máy chỉ nhận Config được cấp.** Không phải máy nào cũng giữ secret prod. Quyền được thực thi bằng mã hóa, không bằng policy mạng.
4. **Local-first.** Đọc và ghi luôn chạy trên bản sao cục bộ, kể cả offline. Thay đổi đồng thời được gộp theo từng key; xung đột hiển thị nhưng không bao giờ chặn `run`.
5. **UX quen thuộc như Doppler**: Project, Environment, Config, Branch config, tham chiếu `${...}`, `run`, `secrets get/set`, `download`.
6. **Cài một dòng, không cần Go.** Binary static cho macOS, Linux, Windows; release có chữ ký.
7. **Thay hẳn `vet45_env`** cho vet45, khách hàng đầu tiên. Dùng chung được cho mọi repo khác.

## 3. Không phải mục tiêu (v1)

- Server lưu secret tập trung, kể cả tự host.
- Tự xây lớp mạng: mesh, NAT traversal, relay qua máy không có Grant. Lớp vận chuyển cắm được đầy đủ (NetBird, ZeroTier…).
- Runner CI ephemeral làm Machine; tích hợp đẩy secret sẵn có kiểu Doppler integrations.
- Grant chỉ đọc; duyệt máy không người; M-of-N Admin.
- Compaction log, purge giá trị, xóa thật Config hay Project.
- Kênh phân phối ngoài `install.sh`/`install.ps1` (Homebrew, Scoop, apt/rpm); codesign và notarize trên macOS.
- Các tính năng riêng của vet45 trong tool cũ: `supabase link`/delegate, `admin create|grant`, đồng bộ env lên Railway, `preActions`, `requireDoctorTargets`, `requireRailwayEnvironment`. kith chỉ quản lý môi trường.
- Nguồn giá trị động (kiểu `supabase status`), template `{{…}}` trong lệnh, khái niệm profile hay workspace.

## 4. Người dùng và kịch bản

| Ai | Cần gì |
|---|---|
| **Dev một mình, nhiều máy** (người dùng vet45 hiện nay) | Laptop, desktop và Mac build cùng thấy secret mới; laptop không giữ secret prod nếu không cần. |
| **Team nhỏ** | Admin cấp Config cho từng máy của từng người; thu hồi khi người rời đi hoặc mất máy. |
| **Máy dùng chung trong tailnet** (Mac build văn phòng, runner tự host) | Là Machine bình thường, thuộc một Member đặt tên theo vai trò (`ci`, `build-mac`). |
| **CI và nền tảng deploy ngoài tailnet** (CircleCI, Railway) | Không biết tới kith. Nhận secret do một Machine tin cậy xuất một chiều vào secret store của nền tảng. |

Kịch bản chính:

- Đứng trong thư mục app, chạy `kith run debug`: Named command `debug` chạy với secret của Active config, Flutter đọc `$KITH_MOUNT`.
- Sửa `DATABASE_URL` trên laptop lúc offline; về mạng thì desktop nhận được trong vài giây.
- Hai máy cùng sửa một key lúc offline: cả hai vẫn chạy được, TUI hiện Conflict, người dùng chọn giá trị đúng.
- Mất laptop: Admin gỡ Machine đó (`lost`), data key được xoay, mọi key nó từng giữ bị gắn cờ cần xoay.
- Mua máy mới: `kith join <invite>`, Admin duyệt với "cấp cùng Grant với Machine cũ".
- Đẩy Config `production` lên Railway: chạy Named command `railway:sync-env` nối `secrets download` vào CLI của Railway.

## 5. Mô hình dữ liệu

### 5.1 Project, Environment, Config

- **Project** gom mọi Environment của một sản phẩm. Mỗi app trong monorepo là một Project riêng; giá trị dùng chung nằm ở một Project chung. Tên Project là slug, không đổi được. Một máy biết hai Project trùng tên thì báo lỗi. Ngoài tên, Project có id riêng (nằm trong header Change).
- **Environment** là một giai đoạn triển khai (`development`, `staging`, `production`…). Mỗi Environment có một root Config.
- **Config** là tập key/giá trị của một Project tại một Environment; là đơn vị được cấp quyền, được mã hóa và được nhân bản.
- **Branch config** kế thừa từ một root Config cùng Environment, một tầng, ghi đè theo từng key (ví dụ `dev_alice` kế thừa `dev`). `unset` trong branch nghĩa là bỏ ghi đè, quay về kế thừa root; v1 không có cách ẩn key của root. Root `unset` một key mà branch đang ghi đè thì giá trị ghi đè vẫn giữ. Config cá nhân chỉ là Branch config được Grant cho các máy của một Member.

### 5.2 Key và giá trị

- Giá trị luôn là văn bản UTF-8, không có NUL. Dữ liệu nhị phân lưu dưới dạng chuỗi base64; không có kiểu giá trị riêng.
- Tối đa **64 KiB** mỗi giá trị, tính theo bytes đã lưu. Vượt thì `secrets set` từ chối. Không có giới hạn tổng theo Config. Giới hạn chỉ được nới, không được hạ.
- Tiền tố `KITH_` là dành riêng. Config có key `KITH_*` thì `run` báo lỗi.
- Giá trị không bí mật theo Environment (`FLAVOR`, `APP_ENV`) là key bình thường.

### 5.3 Tham chiếu

- Cú pháp như Doppler: `${KEY}` (cùng Config), `${config.KEY}` (Config khác cùng Project), `${project.config.KEY}` (Project khác). Phép đổi tên hay dùng chung key giữa các app đều là tham chiếu trong Config, không nằm trong Setup.
- Resolve lúc đọc. Đích nằm ở Config mà máy không có Grant thì `run` dừng và báo lỗi. Grant một Config không tự kéo theo Grant tới các Config nó tham chiếu.
- Lịch sử, rollback, `download` và TUI làm việc trên chuỗi thô; TUI hiện nguyên `${...}`.

### 5.4 Active config

- Config mà một Machine dùng cho một Project khi lệnh không chỉ định. Lưu trong trạng thái cục bộ của máy, **theo Project** (không theo đường dẫn), nên áp dụng cho mọi worktree.
- Thứ tự ưu tiên: `-c <config>` > biến `KITH_CONFIG` > `kith use <config>` > `config` trong Setup.
- `kith use --all <config>` quét `**/kith.yaml` từ gốc git và đặt Active config cho mọi Project có Config trùng tên; Project thiếu Config đó thì được báo, lệnh không dừng.

Nguồn: [Khảo sát mô hình Doppler](https://github.com/nd2204/kith/issues/3), [Mô hình ghi và gộp](https://github.com/nd2204/kith/issues/5), [Định dạng file Setup](https://github.com/nd2204/kith/issues/7), [Secret dạng file](https://github.com/nd2204/kith/issues/26), [ADR 0004](./adr/0004-moi-app-mot-project-setup-khong-bien-doi-key.md).

## 6. File Setup `kith.yaml`

### 6.1 Vị trí và phân giải

- Tên file `kith.yaml`, không ẩn, commit vào repo. Mỗi thư mục app một file.
- kith dùng file gần nhất khi đi ngược lên từ thư mục hiện tại, dừng ở gốc git. Không thấy file nào thì báo lỗi kèm gợi ý `kith setup`.
- Setup lồng nhau không kế thừa nhau: file gần nhất thắng, chỉ cần đọc một file là biết lệnh nào sẽ chạy.
- TUI và `use --all` liệt kê app bằng cách quét `**/kith.yaml` từ gốc git. Không có file workspace.
- `kith run` đọc thẳng file đã commit; không có bước `setup` bắt buộc, không ghi ánh xạ thư mục vào trạng thái máy.

### 6.2 Schema

```yaml
version: 1                      # bắt buộc; version lạ → từ chối, trường lạ → cảnh báo
project: vet45-erp              # slug Project
config: dev                     # Config mặc định
requires: [SUPABASE_URL, SUPABASE_ANON_KEY]   # chỉ tên key
commands:
  debug:
    run: flutter run --flavor "$FLAVOR" --dart-define-from-file="$KITH_MOUNT"
    description: Flutter run with the active flavor
    dir: .                      # tương đối với kith.yaml
    mount: json                 # json | env
  release-android:
    run: flutter build appbundle --flavor prod
    environments: [production]
    files:
      android/app/google-services.json: GOOGLE_SERVICES_JSON
      android/app/upload.jks: { key: ANDROID_KEYSTORE, decode: base64 }
  test: flutter test            # dạng rút gọn
```

Setup **không** lọc, không đổi tên và không đặt giá trị mặc định cho key. Giá trị mặc định là giá trị trong root Config.

### 6.3 Named command

| Trường | Ý nghĩa |
|---|---|
| `run` | Một dòng shell. `sh -c` trên macOS/Linux, PowerShell trên Windows. Không có biến thể theo hệ điều hành, không có template. |
| `description` | Hiển thị trong TUI và bộ chọn inline. |
| `dir` | Thư mục chạy, tương đối với `kith.yaml`; được dùng `..` nhưng không ra ngoài gốc git. Mặc định là thư mục chứa `kith.yaml`. |
| `environments` | Danh sách Environment được phép; so theo Environment nên Branch config cũng thỏa. Vi phạm thì báo lỗi, không có cờ vượt rào. |
| `mount` | `json` hoặc `env`: bật Mount cho lệnh (mục 7.2). |
| `files` | Đường dẫn → key, hoặc `{ key, decode: base64\|none }`: bật Secret file (mục 7.3). Key được trỏ tới coi như nằm trong `requires`. |

- Tên lệnh theo mẫu `[a-z0-9][a-z0-9:_-]*` (ví dụ `deploy-ios:erp`).
- Dạng rút gọn `name: <dòng shell>`.

### 6.4 `kith setup`

Tạo hoặc sửa `kith.yaml` một cách tương tác: hỏi Project và Config, gợi ý `requires`. Không bắt buộc trước `run`.

Nguồn: [Định dạng file Setup và Named command](https://github.com/nd2204/kith/issues/7), [ADR 0004](./adr/0004-moi-app-mot-project-setup-khong-bien-doi-key.md), [ADR 0008](./adr/0008-tuong-thich-phien-ban-bang-khung-header-co-dinh-va-format-floor.md) (trường `version`).

## 7. `kith run` và inject secret

### 7.1 Các dạng gọi

- `kith run <name> [-- args]`: chạy Named command. Tham số sau `--` được shell-escape rồi nối vào cuối dòng `run`.
- `kith run -- <cmd> [args]`: chạy lệnh tùy ý với secret của Config.
- `kith run` không kèm tên: mở bộ chọn **inline** vài dòng ngay dưới prompt (không dùng alt screen). Chọn xong thì bộ chọn biến mất và lệnh chạy như `kith run <name>`.
- Cờ chung: `-c <config>`, `--mount <json|env>` (ghi đè trường `mount`; dùng được cả với `-- <cmd>`), `--allow-missing`, `--sync`.

### 7.2 Trình tự

1. Tìm `kith.yaml` gần nhất; chọn Config theo thứ tự ưu tiên ở mục 5.4.
2. Máy chưa có Grant tới Config thì dừng và chỉ sang luồng xin quyền (mục 10.2).
3. Config đã archive thì dừng, exit khác 0, chỉ cách `kith configs unarchive`.
4. Kiểm `environments`; vi phạm thì dừng.
5. Resolve Config (kế thừa root, tham chiếu). Tham chiếu tới Config không có Grant thì dừng.
6. Kiểm `requires` và các key mà `files` trỏ tới: thiếu thì dừng và liệt kê; `--allow-missing` bỏ qua (Secret file thiếu key thì không ghi file đó).
7. Key trùng tiền tố `KITH_` thì dừng.
8. Có Conflict: dùng giá trị thắng tạm, in cảnh báo ra stderr. Config đang đóng băng do lệch phiên bản: in một dòng cảnh báo ra stderr. Không chặn.
9. Ghi Mount và Secret file (nếu có), xong hết rồi mới chạy tiến trình con.
10. Chạy tiến trình con với env = môi trường hiện tại + Config đã resolve + `KITH_PROJECT`, `KITH_CONFIG`, `KITH_ENVIRONMENT` (+ `KITH_MOUNT` nếu có Mount).
11. Nhận SIGINT/SIGTERM thì chuyển cho tiến trình con và chờ nó thoát.
12. Tiến trình con thoát (bất kể mã thoát) thì xóa Mount và Secret file. Mã thoát và tín hiệu được truyền nguyên vẹn ra ngoài.

**Bất biến**:

- stdout chỉ dành cho tiến trình con. Mọi output của kith, kể cả cảnh báo, đi ra stderr (vì `flutter run --machine` trao đổi JSON qua stdout).
- `run` không bao giờ chờ mạng, trừ khi có `--sync` (mục 11.2). Không cần Agent đang chạy.
- Mount và Secret file là ảnh chụp lúc khởi động; giá trị mới chỉ có hiệu lực khi chạy lại lệnh. Không có `--watch`.

### 7.3 Mount

File tạm chứa toàn bộ Config đã resolve cộng các biến `KITH_*`, đúng tập mà env nhận. Dành cho công cụ đọc secret từ file, điển hình là Flutter `--dart-define-from-file`. Secret **vẫn được inject vào env**; Mount là phần cộng thêm (khác `doppler run --mount`).

- **Định dạng**, phải ghi rõ, không có mặc định:
  - `json`: object phẳng, giá trị luôn là chuỗi. Tài liệu khuyên Flutter dùng `json`.
  - `env`: mỗi dòng `KEY="value"`, dùng chung bộ ghi với `secrets download`.
  - Đuôi file khớp định dạng.
- **File thường, không phải FIFO**: Flutter kiểm bằng `isFileSync` và đọc file nhiều lần lúc khởi động.
- **Vị trí**, không bao giờ trong repo; thư mục 0700, file 0600, tên ngẫu nhiên có chứa pid của `kith run`:
  - Linux: `$XDG_RUNTIME_DIR/kith/` (tmpfs); không có thì `os.TempDir()/kith-<uid>/`.
  - macOS: `$TMPDIR/kith/`.
  - Windows: `%LOCALAPPDATA%\Temp\kith\`, ACL chỉ cấp cho user.
- **Dọn**: xóa khi tiến trình con thoát; không xóa sớm. Mount sót do `kill -9` được lần `run` sau hoặc Agent lúc khởi động dọn, dựa vào pid đã chết.

### 7.4 Secret file

Ghi giá trị của một key ra một đường dẫn cố định mà công cụ cần (keystore Android, `google-services.json`, `.p8` của App Store Connect), chỉ trong lúc lệnh chạy.

- **Đường dẫn**: tương đối với thư mục làm việc của lệnh (`dir`); cho phép `~/…` và đường dẫn tuyệt đối. Ngoài `~` không mở rộng biến nào. Dấu `/` dùng được trên mọi hệ điều hành.
- **`decode`**: `none` (mặc định, ghi nguyên văn bản) hoặc `base64`. Không ghép nhiều key vào một file. Base64 không hợp lệ thì dừng trước khi ghi bất kỳ file nào.
- **Từ chối ghi** (dừng trước khi ghi, báo đường dẫn):
  - Đã có file ở đó mà không phải đồ sót của một lần `run` đã chết (theo sổ). Đồ sót thì dọn rồi chạy tiếp. Không ghi đè, không sao lưu rồi khôi phục; quy tắc này cũng chặn hai `run` song song ghi cùng chỗ.
  - Đường dẫn nằm trong git work tree mà chưa được gitignore (`git check-ignore`): in dòng cần thêm vào `.gitignore`. Không có cờ bỏ qua. Ngoài repo thì không kiểm.
- **Quyền**: file 0600 (Windows: ACL chỉ user); thư mục cha chưa có thì tạo với 0700. Ghi hỏng giữa chừng thì gỡ những file đã ghi và dừng.
- **Sổ**: trước khi ghi, kith ghi vào thư mục trạng thái: pid, đường dẫn, hash nội dung, các thư mục đã tạo.
- **Dọn**: vòng đời như Mount. Chỉ xóa file khi hash còn khớp; file đã bị sửa thì giữ lại và cảnh báo. Chỉ xóa thư mục do kith tạo, khi đã trống. Đồ sót do `kill -9` dọn như Mount.
- Key mà `files` trỏ tới vẫn có trong env và Mount (Setup không biến đổi key).
- Không có cờ `run --file` ở v1. Với `run -- <cmd>` tùy ý, tài liệu có công thức `base64 -d` trong dòng shell.

### 7.5 Lưu ý bảo mật cho Flutter, và quy tắc tách Project deploy

flutter_tools mã hóa dart-define bằng base64 (đảo ngược được), ghi vào `ios/Flutter/Generated.xcconfig`, `flutter_export_environment.sh`, dòng lệnh Gradle, và nhúng vào bản build. Vì vậy:

- Config của app Flutter chỉ chứa key phía client.
- Secret dùng để ký và upload (keystore, `.p8`, key App Store Connect) nằm ở một Project deploy riêng; Named command của Project đó trỏ `dir:` về app.
- Named command có cả `mount: json` và `files:` thì `run` cảnh báo.

### 7.6 Windows

- Dòng `run` chạy trong PowerShell, nên phải viết `$env:KITH_MOUNT`. Setup viết cho Unix không chạy trên Windows; kith không tự thay `$KITH_*`. Tài liệu ghi rõ giới hạn này (Windows là tier 2).
- Giá trị dài hơn 32.767 ký tự: `run` cảnh báo và bỏ key đó khỏi env; Secret file vẫn được ghi.

### 7.7 IDE

Không thêm tính năng. Tài liệu có công thức cho VS Code (Dart-Code): Named command `debug` như trên; wrapper `exec kith run debug -- "$@"`; trong `launch.json`, `customTool` trỏ tới wrapper và `customToolReplacesArgs: 1`. Không làm được thì dùng `download`.

### 7.8 Session nhúng trong TUI

Chạy Named command trong TUI dùng đúng các quy tắc trên (Mount, Secret file, tín hiệu, dọn), trong một PTY.

Nguồn: [Inject secret cho công cụ cần file](https://github.com/nd2204/kith/issues/6), [Secret dạng file ghi ra đường dẫn cố định](https://github.com/nd2204/kith/issues/26), [Định dạng file Setup](https://github.com/nd2204/kith/issues/7), [Bố cục TUI](https://github.com/nd2204/kith/issues/9).

## 8. Thao tác secret

| Lệnh | Hành vi |
|---|---|
| `kith secrets get <KEY>` | In giá trị (đã resolve). Chạy cả với Config đã archive. Có Conflict thì cảnh báo stderr. |
| `kith secrets set <KEY> [value]` | Ghi một Change cục bộ, luôn thành công kể cả offline. Xong thì nudge Agent; Agent không chạy thì báo "not synced". |
| `kith secrets set <KEY> --from-file <path\|-> [--base64]` | Lưu nguyên nội dung file. Không phải UTF-8 hoặc có NUL thì từ chối và gợi ý `--base64`. `--base64` mã hóa trước khi lưu; 64 KiB tính sau khi mã hóa. Không tự đoán định dạng. |
| `kith secrets unset <KEY>` | Change xóa key (trong branch: bỏ ghi đè). |
| `kith secrets download --format env\|json [--out <path>]` | Ảnh chụp một lần, mặc định ra stdout. `--out` ghi file 0600, cảnh báo nếu chưa gitignore. Không có chế độ giữ file luôn mới, không có cờ lọc key. |
| `kith secrets upload <file.env\|json>` | Nạp hàng loạt key từ file vào Config. Dùng chung cho mọi repo chuyển từ `.env`. |
| `kith conflicts` | Liệt kê các Conflict. |
| `kith secrets resolve <KEY>` | Chọn một giá trị đang tranh hoặc nhập giá trị mới; ghi Change mới trỏ tới mọi head đang xung đột. |
| `kith secrets flag <KEY>` / `--all` | Gắn cờ "cần xoay secret" bằng tay. |
| `kith secrets unflag <KEY>` | Gỡ cờ mà không đổi giá trị. |
| `kith secrets history <KEY>` | Mục 13.2. |
| `kith secrets rollback <KEY> [--to <change> \| --at <time>]` | Mục 13.3. |
| `kith secrets reapply <change>` | Áp dụng lại một Change bị loại (mục 13.3). |

Mọi thao tác ghi bị từ chối với Config đã archive. Mọi lệnh hiển thị giá trị che giá trị mặc định trừ khi có `--reveal` (lịch sử) hoặc đang `get` đúng key.

Nguồn: [Mô hình ghi và gộp](https://github.com/nd2204/kith/issues/5), [Inject secret cho công cụ cần file](https://github.com/nd2204/kith/issues/6), [Kế hoạch chuyển](https://github.com/nd2204/kith/issues/10), [Compaction và giữ lịch sử](https://github.com/nd2204/kith/issues/24), [Xem lịch sử và rollback](https://github.com/nd2204/kith/issues/25), [Secret dạng file](https://github.com/nd2204/kith/issues/26).

## 9. Danh tính, quyền và mã hóa

### 9.1 Machine và khóa

- Chủ thể được cấp quyền là **Machine**. Mỗi máy có một cặp khóa của kith, dùng chung cho mọi Project:
  - **age X25519** (`filippo.io/age`) để nhận data key của Config.
  - **Ed25519** để ký mọi Change và làm chứng chỉ mTLS.
- Private key nằm trong kho khóa của hệ điều hành, qua thư viện thuần Go (không CGO): `security` trên macOS, Secret Service qua D-Bus trên Linux, wincred trên Windows. Máy không có kho khóa thì lưu ở file 0600 và cảnh báo.
- **Member** là nhãn gom các Machine của một người (hoặc một vai trò như `ci`); không có khóa.

### 9.2 Project là phạm vi tin cậy

- Mỗi Project là một phạm vi tin cậy riêng; v1 không có Workspace chung.
- Danh sách Member, Machine (kèm public key) và bảng Grant là **dữ liệu của kith**, nằm trong log Project, được nhân bản tới mọi Machine của Project và chỉ hợp lệ khi có chữ ký hợp lệ. Mọi Machine trong Project đều thấy chúng.
- Không dựa vào policy tailnet (ACL, grants, CapMap): tag xóa danh tính user, Headscale không có Tailnet Lock, và ai sửa được policy tailnet thì cấp được quyền. `WhoIs` chỉ là kiểm tra phụ.

### 9.3 Admin

- Vai trò của một Member trong một Project. Mọi thay đổi về thành viên, Grant, archive và Format floor phải do một Machine của Member Admin ký. Ngoại lệ duy nhất: một Machine được tự gỡ chính nó.
- Project luôn có ít nhất một Admin; kith từ chối gỡ Machine cuối cùng có quyền Admin.
- Duyệt một máy vào Member là Admin cũng là trao quyền ký Admin cho máy đó; TUI nói rõ điều này lúc duyệt.

### 9.4 Grant

- Grant là quyền **đọc và ghi** của một Machine với một Config. Không có Grant chỉ đọc ở v1.
- Grant một Branch config bao gồm quyền đọc root của nó.
- Cấp quyền và bọc khóa là hai bước tách rời: Admin ký lệnh Grant trong log Project; tiến trình kith đầu tiên trên một Machine đang giữ Config thấy lệnh đó sẽ bọc data key cho máy mới. Nhờ vậy Admin không cần giữ secret.
- Máy mới vào Project không có Grant nào.

### 9.5 Mã hóa end-to-end theo Config

- Mỗi Config có một data key. Data key được bọc (stanza age) tới public key X25519 của từng Machine có Grant.
- Payload của từng Change (tên key, giá trị, HLC) mã hóa bằng data key với **XChaCha20-Poly1305**, nonce ngẫu nhiên 24 byte. Tên key cũng được mã hóa; metadata gộp không làm lộ tên key.
- Ciphertext của Config chỉ được nhân bản giữa các Machine có Grant; máy khác không giữ hộ, không chuyển tiếp.

### 9.6 Thu hồi

- Thu hồi Grant là một Change trong log Project, có hiệu lực ngay về mặt nhân bản: các Machine ngừng gửi Config cho máy đó.
- Data key được xoay (epoch mới) bởi tiến trình kith đầu tiên trên một Machine giữ Config thấy lệnh thu hồi. Trong lúc chờ, TUI hiện "revocation pending key rotation".
- Giá trị cũ coi như đã lộ: kith gắn cờ "cần xoay secret" cho các key của Config đó và liệt kê chúng, nhưng không ép và không chặn `run`.

### 9.7 Mô hình kẻ tấn công

- Machine có Grant được tin về giá trị (data key đối xứng cho phép nó ghi bất kỳ giá trị nào), nhưng không đóng giả được máy khác, và không ký lại được Change của máy khác (kể cả máy đã thu hồi) mà không bị phát hiện.
- Machine trong Project không có Grant, hoặc máy đã bị thu hồi (vẫn giữ key các epoch cũ), không tạo được Change hợp lệ ngoài quy tắc quá khứ nhân quả.
- Bên ngoài Project, kể cả người có ciphertext từ đĩa, từ file backup hay trên đường truyền, không đọc được, không tạo được Change, và không ghép được Change vào chỗ cha, Config hay epoch khác.
- Đọc không để lại dấu vết: lịch sử chỉ chứa thao tác ghi. Đây là hệ quả của local-first, tài liệu ghi rõ.

Nguồn: [Chọn lớp mạng riêng](https://github.com/nd2204/kith/issues/2), [Mô hình cấp, thu hồi quyền](https://github.com/nd2204/kith/issues/4), [Ràng buộc tác giả vào Change](https://github.com/nd2204/kith/issues/14), [ADR 0001](./adr/0001-machine-la-recipient-quyen-trong-du-lieu-kith.md), [ADR 0002](./adr/0002-age-x25519-va-ed25519.md).

## 10. Vòng đời Machine

### 10.1 Khởi tạo

`kith init` sinh khóa Machine nếu chưa có và hỏi có cài Agent không (mặc định có). Xem mục 19 về phạm vi chính xác của lệnh này và việc tạo Project.

### 10.2 Invite và join

- `kith invite <project> [--ttl <dur>]` (Admin) in một **Invite**: mã gồm id Project, tên MagicDNS và fingerprint khóa của Machine Admin, có trường version. TTL mặc định 24 giờ. Dùng được nhiều lần trong thời hạn; `kith invite --revoke` hủy sớm. Invite tự nó không cấp quyền gì.
- `kith join <mã>`: sinh khóa nếu chưa có, in fingerprint, nối tới Agent của Admin và kiểm chứng chỉ server với fingerprint trong mã, rồi gửi yêu cầu. Hỏi có cài Agent không.
- Agent của Admin nhận client lạ ở đúng một endpoint `join`, chỉ cho máy mang Invite hợp lệ, và giới hạn số yêu cầu đang chờ cho mỗi mã. Muốn duyệt thì Agent của Admin phải đang chạy.
- Admin duyệt trong TUI: đối chiếu fingerprint và `WhoIs`, gán máy vào một Member (có sẵn hoặc mới), rồi ký một Change. Tùy chọn trong cùng Change: "grant the same as Machine X" và "also remove Machine X".
- Máy cũ không tự duyệt được máy mới của cùng Member (chặn kẻ trộm máy cũ thêm máy của họ).

### 10.3 Gỡ Machine

- `kith machines remove <machine>`: **một Change** trong log Project. Máy ra khỏi Project, mọi Grant mất theo, mỗi Config máy đó từng giữ được xoay data key như thu hồi.
- Admin gỡ được mọi Machine, luôn với lý do `lost`. Một Machine tự gỡ chính nó với lý do `lost` hoặc `retired`.
  - `lost`: xoay data key và gắn cờ "cần xoay secret" cho mọi key của các Config máy đó từng giữ.
  - `retired`: kith xóa khóa và bản sao cục bộ trên máy; vẫn xoay data key nhưng không gắn cờ.
- Chữ ký cũ theo quy tắc nhân quả của thu hồi (mục 12.4): Change nằm trong quá khứ nhân quả của lệnh gỡ vẫn hợp lệ; Change đồng thời hoặc đến sau bị loại nhưng vẫn hiển thị. Quy tắc này cũng chặn kẻ trộm ký Change mới trỏ tới cha cũ.
- Public key của máy đã gỡ bị cấm vĩnh viễn trong Project; muốn vào lại phải `join` với khóa mới.
- `kith members remove <member>`: một Change gỡ mọi Machine của Member với lý do `lost`, và bỏ vai trò Admin nếu có.

### 10.4 Recovery machine và khôi phục Admin

- `kith machines create --recovery`: tạo một Machine của một Member Admin, xuất khóa ra để cất offline. Máy này không chạy kith thường xuyên. Không có luật gộp mới: quyền ký Admin đến từ vai trò của Member.
- `kith recover`: nạp khóa Recovery machine vào bộ nhớ, ký một Change duyệt máy hiện tại vào Member Admin (hoặc phong Admin cho một Member), rồi bỏ khóa đi, không ghi xuống đĩa.
- Lộ khóa Recovery machine thì gỡ nó với lý do `lost` và tạo cái mới.
- Lối thoát cuối cùng khi không có Recovery machine: các máy còn Grant tạo Project mới rồi nạp lại dữ liệu; thành viên và Grant phải dựng lại.

### 10.5 Sao lưu

- Recovery machine có thể nhận Grant (bọc data key chỉ cần public key). Mỗi lần xoay, máy online tự bọc khóa mới cho nó.
- `kith backup export`: xuất các Change đã mã hóa của những Config được chọn cùng các data key đã bọc ra một file có trường version. Người dùng tự cất. Không có lịch sao lưu tự động.
- `kith backup import`: khôi phục trên một Machine có Grant, hoặc bằng khóa Recovery machine. Một file cũ chỉ giải mã được tới epoch mà nó chứa.

### 10.6 Cảnh báo

- Project chỉ có một Machine có quyền Admin (Recovery machine được tính).
- Config chỉ có một bản sao (chỉ đếm Machine thường đang giữ ciphertext; không đếm Recovery machine, file sao lưu, và bỏ qua Config đã archive).

### 10.7 Gỡ cài đặt

Không có lệnh riêng. Tài liệu ghi ba bước: gỡ Machine khỏi các Project với lý do `retired`; `kith agent uninstall`; xóa binary, kèm đường dẫn kho cục bộ và mục trong kho khóa nếu muốn xóa sạch.

Nguồn: [Vòng đời Machine](https://github.com/nd2204/kith/issues/11), [Tiến trình đồng bộ](https://github.com/nd2204/kith/issues/13), [Mô hình cấp, thu hồi quyền](https://github.com/nd2204/kith/issues/4), [Cài đặt và phát hành](https://github.com/nd2204/kith/issues/8), [ADR 0005](./adr/0005-khoi-phuc-admin-bang-recovery-machine.md).

## 11. Đồng bộ và Agent

### 11.1 Agent

- Mỗi Machine chạy một **Agent** (`kith agent`), service cấp user: LaunchAgent trên macOS, `systemd --user` trên Linux, Scheduled Task lúc đăng nhập trên Windows. Không dùng system service (khóa nằm trong kho khóa của user; không cần quyền admin khi cài).
- Agent là tiến trình **duy nhất** của kith giữ mạng; một Agent phục vụ mọi Project trên máy.
- `kith agent install` / `kith agent uninstall`. `install.sh`/`install.ps1` và `self-update` khởi động lại Agent sau khi nâng cấp binary.
- Agent không bao giờ chạy Named command.

### 11.2 Lệnh và bản sao cục bộ

- `run`, `secrets` và TUI chỉ đọc và ghi bản sao cục bộ, không cần Agent.
- Lệnh ghi xong thì nudge Agent qua IPC cục bộ (Unix socket 0600, hoặc named pipe có ACL của user). IPC chỉ mang nudge và truy vấn trạng thái, **không bao giờ mang secret**.
- `--sync`: kéo trước với timeout ngắn; hết timeout thì vẫn đọc bản cục bộ và cảnh báo.
- `kith sync`: đồng bộ một lần ở foreground (nối tới peer đang online, trao đổi log, thoát), cho máy không có Agent.

### 11.3 Khi nào đồng bộ

Agent đồng bộ khi khởi động, khi nhận nudge, khi một peer vừa online (theo dõi IPN bus qua LocalAPI), và định kỳ mỗi 5 phút (chỉnh được). Đẩy bằng cách báo head mới cho các peer đang online; peer tự kéo về.

### 11.4 Peer và kết nối

- Lớp mạng v1: tailnet có sẵn (Tailscale hoặc Headscale) qua `tailscale.com/client/local`. Code đặt sau một seam `Transport` hẹp để sau này cắm `tsnet`.
- Địa chỉ peer là trạng thái mềm cục bộ, không nằm trong log Project: lấy lúc join (stable NodeID, tên MagicDNS qua `WhoIs`), các Agent báo cho nhau khi đồng bộ, đổi thành IP qua LocalAPI `Status`.
- Agent chỉ bind vào IP tailnet, ở một cổng cố định chỉnh được. tailscaled chưa chạy thì chờ và bind lại khi IP xuất hiện.
- Xác thực hai chiều bằng **mTLS** với chứng chỉ tự ký từ khóa Ed25519 của Machine, đối chiếu với danh sách Machine trong log Project. `WhoIs` chỉ là kiểm tra phụ.
- Log Project đồng bộ với mọi Machine chưa bị gỡ. Log Config chỉ đồng bộ với Machine có Grant (theo bản nhìn cục bộ). Full mesh tới mọi peer liên quan đang online.
- Không có relay. Các máy không cùng online thì nhân bản bị trễ; tài liệu khuyên có một Machine luôn bật có Grant.

### 11.5 Giao thức v1

- HTTP/2 trên mTLS, ALPN `kith/1` (mục 14.3).
- Mỗi log là một đơn vị đồng bộ. Hai bên trao đổi head, rồi fetch-by-hash đi ngược theo hash cha, theo lô. Header đọc được mà không cần giải mã.
- Change nhận về được kiểm hash, chữ ký và quyền nhân quả trước khi nhập kho. Change bị loại vẫn được lưu và nhân bản.
- Negotiation kiểu want/have để sau, khi đã đo được log lớn.

### 11.6 Kho cục bộ

- Kho đối tượng kiểu git: mỗi Change là một file bất biến đặt tên theo hash, ghi bằng file tạm rồi rename nguyên tử.
- File heads/index của từng log được khóa ngắn bằng `flock` / `LockFileEx`, cho nhiều tiến trình kith cùng ghi.
- Trạng thái tính sẵn chỉ là cache; mất thì dựng lại từ log. Kho có số schema (mục 14.5).

Nguồn: [Tiến trình đồng bộ](https://github.com/nd2204/kith/issues/13), [Chọn lớp mạng riêng](https://github.com/nd2204/kith/issues/2), [ADR 0006](./adr/0006-dong-bo-qua-agent-cap-user-va-mtls.md).

## 12. Log Change, gộp và Conflict

### 12.1 Log

- Mỗi Config, và phần thành viên, Grant, archive, Format floor của mỗi Project, là một log **Change** chỉ-thêm, dạng DAG giống git: mỗi Change do một Machine ký và trỏ tới hash của các head mà máy đó đã thấy.
- Gộp là hợp hai tập Change; trạng thái hiện tại được tính lại từ log.
- Quan hệ nhân quả do DAG quyết định. **HLC** (trong payload) chỉ dùng để xếp các Change đồng thời. HLC tăng dọc theo nhân quả.
- Branch config có log riêng, tách khỏi log root.
- Log Project không mã hóa bằng data key, chỉ nhân bản giữa các Machine trong Project.

### 12.2 Khung Change

- **Header**, khung cố định suốt kith 1.x: `format`, id Project, id Config, epoch data key, Machine tác giả, các hash cha. Sau khung là vùng extension TLV, mỗi TLV có bit `critical`.
- Header được truyền và lưu dưới dạng **byte nguyên vẹn**, không bao giờ dựng lại từ các trường đã parse.
- Payload Config: XChaCha20-Poly1305, AAD = đúng các byte của header.
- `hash = SHA-256(header ‖ nonce ‖ ciphertext)`.
- Chữ ký Ed25519 của tác giả ký lên hash kèm tiền tố phân tách miền `kith/change/v1`. Log Project dùng cùng khung (byte header nguyên vẹn, hash, chữ ký), không có ciphertext, tiền tố `kith/project-change/v1`.
- Ký lại hoặc ghép lại ciphertext dưới header khác thì giải mã thất bại. Phương án ký cả trong lẫn ngoài bị loại.

### 12.3 Kiểm Change khi nhận

- Hash, chữ ký và quyền được kiểm không cần key.
- Máy có data key giải mã thử ngay lúc nhập. Máy chưa có key (vừa được Grant, hoặc đang `backup import`) đánh dấu "chưa kiểm nội dung" và kiểm khi key tới. Kết quả kiểm chỉ nằm trong cache.
- Change có chữ ký hợp lệ mà **không giải mã được** thì bị loại: vẫn là nút trong DAG, vẫn được nhân bản, nhưng không đóng góp giá trị, không tạo Conflict. TUI và `doctor` cảnh báo đích danh Machine đã ký; không có nút áp dụng lại; kith không tự thu hồi máy đó.

### 12.4 Quyền xét theo quá khứ nhân quả

- Một Change chỉ hợp lệ nếu tác giả có Grant (log Config) hoặc là Admin (log Project), **xét theo các Change cha của nó**.
- Hai Admin thu hồi lẫn nhau đồng thời: Admin thâm niên cao hơn thắng (thứ tự nhân quả trong log Project; hòa thì so HLC rồi public key).
- Grant và thu hồi cùng Grant đó đến đồng thời: thu hồi thắng.
- Change của máy bị thu hồi hoặc bị gỡ mà không nằm trong quá khứ nhân quả của lệnh đó thì bị **loại**, nhưng vẫn hiển thị ("N Changes rejected from revoked machine") để người có Grant áp dụng lại bằng tay (`reapply`).

### 12.5 Gộp theo từng key và Conflict

- Hai Change đồng thời chạm các key khác nhau thì gộp tự động.
- Hai Change đồng thời đặt khác giá trị cho cùng một key (kể cả `set` và `unset` đồng thời) tạo một **Conflict**. kith chọn tạm giá trị theo quy tắc tất định: HLC lớn hơn thắng, hòa thì so public key Machine. `run` không bao giờ bị chặn.
- Conflict hiển thị (stderr của `run`/`get`, `kith conflicts`, huy hiệu TUI) cho tới khi có người giải bằng `secrets resolve`, rollback, hoặc màn hình Conflict trong TUI. Kết quả là một Change mới có cha là mọi head đang xung đột.

### 12.6 Epoch data key

- Mỗi lần xoay tạo một epoch mới; key của epoch mới bọc luôn key của epoch trước. Mọi Machine đang có Grant, kể cả máy được cấp về sau, đọc được toàn bộ lịch sử; máy đã bị thu hồi không đọc được epoch mới.
- Change ghi bằng key cũ bởi máy chưa thấy lệnh xoay vẫn được chấp nhận, miễn tác giả không bị thu hồi.
- Xoay và bọc là Change cục bộ, do tiến trình kith đầu tiên thấy lệnh thu hồi hoặc Grant thực hiện (Agent, `kith sync` hay một lệnh bất kỳ). Không có vai trò riêng.
- Hai lần xoay đồng thời tạo hai epoch đồng thời, cả hai hợp lệ. Lần ghi sau chọn epoch thắng theo HLC rồi public key; Change mới của epoch thắng bọc luôn key của epoch thua.

Nguồn: [Mô hình ghi và gộp](https://github.com/nd2204/kith/issues/5), [Ràng buộc tác giả vào Change](https://github.com/nd2204/kith/issues/14), [Tiến trình đồng bộ](https://github.com/nd2204/kith/issues/13), [ADR 0003](./adr/0003-log-change-co-chu-ky-dang-dag.md).

## 13. Lịch sử, rollback và archive

### 13.1 Giữ lịch sử vĩnh viễn

- v1 không có compaction, không có purge, không xóa thật Config hay Project. Log được giữ đầy đủ trên mọi Machine giữ chúng.
- Mọi giá trị từng được `set` nằm vĩnh viễn trong lịch sử và đọc được bởi mọi Machine có Grant, hiện tại và tương lai. Tài liệu ghi rõ. Lỡ ghi nhầm secret thì cách duy nhất là xoay ở nguồn.
- **Cờ "cần xoay secret"**: gắn tự động khi thu hồi hoặc gỡ Machine `lost`, hoặc bằng tay qua `secrets flag`. Ai có Grant cũng ghi được `flag`/`unflag` (đều là Change trong log Config). Cờ hết khi có một `set` key đó nằm sau cờ theo nhân quả. Rollback và `reapply` không tính là `set`. Không có cờ cho cả Config. Khi cờ được gắn, tài liệu nhắc chạy lại các Named command xuất (mục 16).
- `doctor` và TUI hiện số Change và dung lượng từng log; `doctor` cảnh báo vàng khi một log vượt 50.000 Change hoặc 50 MB (chỉnh được bằng cấu hình cục bộ), kèm lời khuyên mở issue.
- Change bị loại và Change không đọc được được giữ mãi và vẫn nhân bản. "Dismiss" trong TUI chỉ là đánh dấu cục bộ. Không có GC.
- Muốn bỏ dữ liệu khỏi một máy thì gỡ Grant hoặc gỡ Machine.

### 13.2 Xem lịch sử

- `kith secrets history <KEY>`: dòng thời gian của một key. Mỗi dòng: short hash, thời điểm HLC, Machine tác giả kèm nhãn Member, loại thao tác (`set`, `unset`, `resolve`, `rollback`, `reapply`, `flag`, `unflag`). Nhánh đồng thời từng gây Conflict được đánh dấu. Key là tham chiếu thì ghi chú "value is a reference" và chỉ sang key nguồn.
- Trên Branch config: dòng thời gian hiệu lực, ghép log branch và log root theo HLC, có cột nguồn (`dev_alice` / `↳ dev`). Ghép chỉ để hiển thị.
- `kith configs log [config]`: kiểu `git log`, mỗi Change một dòng, chỉ có tên key bị chạm; xen vào (hiện mờ) các mốc liên quan từ log Project: Grant/thu hồi trên Config, xoay epoch, archive.
- `kith projects log [project]`: log Project đầy đủ.
- Giá trị che mặc định, `--reveal` để hiện. Thứ tự: topo của DAG, Change đồng thời xếp theo HLC rồi public key.
- Change bị loại hiện mờ với nhãn `rejected`; Change không đọc được hiện `unreadable (format N)`, không có tên key.
- Short hash kiểu git: tối thiểu 7 ký tự hex, tự dài thêm khi trùng. Không có cú pháp tương đối kiểu `HEAD~2`.

### 13.3 Rollback và reapply

- **Rollback** là một Change thường gồm `set`/`unset`, kèm dấu `rollback` trong payload trỏ tới hash Change nguồn ("rolled back to a1b2c3d"). Không xóa hay viết lại lịch sử. Ai có Grant thì rollback được.
- Phạm vi:
  - Một key: `kith secrets rollback <KEY> [--to <change>]`. Không có `--to` thì về giá trị liền trước của key theo thứ tự hiển thị.
  - Cả Config: `kith configs rollback <config> --to <change>`. "Trạng thái tại X" tính từ quá khứ nhân quả của X. Ghi **một** Change chứa toàn bộ khác biệt.
  - Theo thời điểm: `--at <time>` (tập Change có HLC ≤ t luôn đóng theo nhân quả), tính trên bản sao cục bộ.
- Mọi rollback in phần khác biệt rồi hỏi xác nhận; `--yes` để bỏ qua.
- **Conflict**: Change rollback có cha là mọi head hiện tại nên giải Conflict của các key nó chạm. Rollback cả Config ghi tường minh mọi key đang Conflict, kể cả khi giá trị đích trùng giá trị thắng tạm; CLI báo "resolves N conflicts". Hai máy rollback đồng thời thì thành Conflict bình thường.
- **Cờ**: rollback không gỡ cờ. Giá trị đích từng bị gắn cờ thì Change rollback tự gắn lại cờ cho key đó trong cùng Change; CLI và TUI cảnh báo trước ("STRIPE_KEY: restoring a value flagged for rotation"), không chặn.
- **Branch config**: rollback trên branch chỉ ghi vào log branch. Rollback về lúc key đang kế thừa nghĩa là `unset` override (kế thừa giá trị **hiện tại** của root); CLI nói rõ. `configs rollback` trên branch chỉ đụng override; rollback root không đụng override của branch.
- `--to` không nhận Change bị loại. `kith secrets reapply <change>` ghi một Change mới với các key của Change bị loại, đánh dấu `reapply`. Quy tắc cờ như rollback.
- Rollback khôi phục chuỗi thô (`${...}`), không phải giá trị đã resolve.
- Log Project không có rollback; Grant và archive có lệnh ngược riêng.

### 13.4 Archive

- Chỉ Admin archive/unarchive được, bằng Change trong log Project: `kith configs archive|unarchive <config>`, `kith projects archive|unarchive <project>`, `kith configs list --archived`.
- Archive root Config thì archive luôn các Branch config của nó; archive Project thì archive mọi Config.
- Unarchive chỉ bật lại những gì bị archive bởi đúng Change đó. Không unarchive được Branch config khi root còn archive.
- Với Config đã archive: `run`, `use`, `secrets set/unset`, `download` và rollback từ chối, exit khác 0, chỉ cách unarchive. `secrets get`, xem lịch sử và `backup export` vẫn chạy.
- Máy offline chưa thấy lệnh archive vẫn chạy bình thường; Change ghi vào Config đã archive vẫn hợp lệ và hiện lại khi unarchive.
- Nhân bản, Grant, xoay key, cờ và Conflict hoạt động như thường. Ngoại lệ duy nhất: bỏ cảnh báo "chỉ còn một bản sao".
- `doctor` liệt kê các Setup và Active config đang trỏ tới Config đã archive.

Nguồn: [Compaction và giữ lịch sử](https://github.com/nd2204/kith/issues/24), [Xem lịch sử và rollback](https://github.com/nd2204/kith/issues/25), [ADR 0009](./adr/0009-giu-lich-su-vinh-vien-archive-thay-cho-xoa.md).

## 14. Tương thích phiên bản

### 14.1 `format` và extension

- `format` là số nguyên tăng dần, chỉ báo cách mã hóa payload và nghĩa của extension; không đổi khung header. Loại extension mới chỉ đi kèm một `format` mới.
- Extension lạ có bit `critical` thì Change coi như không đọc được; không có `critical` thì bỏ qua.
- Tiền tố chữ ký `kith/change/v1` và `kith/project-change/v1` đánh version cho khung hash và chữ ký, cố định suốt 1.x; đổi chúng là lên kith 2.

### 14.2 Format floor

- Bản mới **không tự ghi** `format` mới. Nó ghi theo **Format floor** của Project.
- Admin nâng sàn bằng `kith projects upgrade` (Change trong log Project; loại Change này có từ v1.0). Sàn áp dụng cho log Project và mọi Config, chỉ tăng; hai lần nâng đồng thời thì lấy giá trị lớn hơn. Machine của Admin phải ghi được format đó.
- Lệnh liệt kê Machine chưa gỡ theo ba nhóm (ready / not ready / never seen) dựa trên `hello` gần nhất, rồi bắt xác nhận. Không chặn cứng.
- Change có format dưới sàn mà quá khứ nhân quả chứa lệnh nâng sàn thì bị loại (vẫn hiển thị, áp dụng lại được). Change đồng thời với lệnh nâng vẫn hợp lệ.
- Bản kith không ghi được format của sàn thì từ chối ghi vào Project đó và nhắc `kith self-update`; đọc và `run` vẫn chạy.

### 14.3 Máy cũ gặp Change không đọc được

- Vẫn lưu và nhân bản. Trạng thái tính trên phần log khép kín nhân quả, bỏ Change đó **và mọi Change con cháu**. Config đóng băng như lúc offline: cũ nhưng nhất quán, `run` không bị chặn (cảnh báo stderr).
- Log Project xử lý giống vậy; máy cũ có thể chưa thấy lệnh thu hồi mới (chấp nhận được, vì máy bị thu hồi không có data key epoch mới). `doctor` cảnh báo đỏ.

### 14.4 Giao thức đồng bộ

- Major trong ALPN (`kith/1`). Không có major chung thì TLS bắt tay thất bại; `doctor` báo lỗi đích danh peer.
- Endpoint `hello` trao version kith, dải minor giao thức `[min, max]` và `format` cao nhất đọc được. Hai bên dùng minor chung cao nhất; kết quả lưu vào sổ địa chỉ mềm.
- Minor chỉ được thêm (endpoint mới, trường tùy chọn); bỏ hay đổi nghĩa phải lên `kith/2`.
- Chính sách: mọi bản 1.x đọc được mọi `format` cũ và nói chuyện được với nhau. Không có quy tắc "N-1 minor". Điểm cắt cứng duy nhất là Admin nâng sàn.

### 14.5 Định dạng khác

- Kho cục bộ có số schema: bản mới tự migrate; bản cũ gặp schema mới hơn thì dựng lại cache (Change là file bất biến), không được thì từ chối và nhắc `self-update`.
- File `backup export` và mã Invite có trường version; gặp version lạ thì từ chối kèm "requires kith ≥ vX".
- `kith.yaml` có `version: 1`.

Nguồn: [Tương thích phiên bản](https://github.com/nd2204/kith/issues/20), [ADR 0008](./adr/0008-tuong-thich-phien-ban-bang-khung-header-co-dinh-va-format-floor.md).

## 15. TUI

`kith` không kèm tham số mở TUI Bubble Tea, bố cục panel kiểu lazygit. Bản nháp tham khảo: nhánh [`prototype/tui-layout`](https://github.com/nd2204/kith/tree/prototype/tui-layout/prototypes/tui-layout), biến thể A ([`screens.txt`](https://github.com/nd2204/kith/blob/prototype/tui-layout/prototypes/tui-layout/screens.txt)). Giao diện tiếng Anh.

### 15.1 Bố cục và phím

- Trái: panel xếp chồng `[1] Projects`, `[2] Configs`, `[3] Commands`, `[4] Machines`. Phải: panel chính `[0]` và dải Log. Panel chính đổi theo panel đang focus: secret của Config, chi tiết hoặc session của Named command, Grant của Machine.
- Màn hình phụ (sửa giá trị, xử lý Conflict, xác nhận, lịch sử) chiếm panel chính, không mở cửa sổ đè.
- Phím chung: `tab` đổi panel, `1-4`/`0` nhảy tới panel, `j/k` di chuyển, `s` đồng bộ, `q` thoát. Một phím bật hiện Config/Project đã archive (mờ, nhãn `archived`; ẩn mặc định).
- Panel secret: `v`/`V` hiện giá trị, `e` sửa, `a` thêm, `d` unset (hỏi lại), `enter` trên key đang Conflict mở màn hình Conflict, `h` lịch sử key, `L` log của Config, `R` reapply Change bị loại.
- Màn hình lịch sử/log: `j/k`, `v`, `enter` xem các key một Change chạm, `r` rollback về Change đang chọn (màn hình xác nhận có diff và cảnh báo cờ), `R` reapply Change `rejected`, `esc` quay lại.

### 15.2 Config

- Chọn Config trong panel **không** đổi Active config. Active config có dấu `●`, đổi bằng `space` (tương đương `kith use`).
- Branch config thụt dưới root (`↳ dev_alice`). Key kế thừa ghi `↳ dev`, key ghi đè ghi `override`, tham chiếu hiện nguyên `${...}`. TUI hiện lần đồng bộ cuối của từng Config.

### 15.3 Việc cần người xử lý

- Huy hiệu trên Config: `⚠N` Conflict, `⟳` thu hồi đang chờ xoay khóa, `✖N` Change bị loại, `no Grant`.
- Banner đầu panel secret: Conflict; Change bị loại kèm `R`; chờ xoay khóa kèm số key cần xoay; key thiếu so với `requires`; Config đóng băng. Key cần xoay có cờ `⟳ rotate`.
- Màn hình Conflict: các giá trị đang tranh kèm Machine và thời điểm, đánh dấu giá trị `run` đang dùng, lựa chọn "enter a new value…", lịch sử key bên dưới.
- Cảnh báo cấp Project: chỉ một Machine Admin; Config chỉ một bản sao; Agent không chạy. Footer hiện "kith vX available".
- Mọi sự kiện ghi vào Log.

### 15.4 Machines và cấp quyền

- Panel Machines hiện trạng thái từng máy: online, số Change còn thiếu, offline từ bao giờ, đã thu hồi, đang xin join, version kith (thông tin khi peer chạy bản cũ).
- `enter` trên một máy mở danh sách Project/Config dạng `[x]`; `space` cấp hoặc thu hồi (Change có chữ ký Admin). Không dùng ma trận.
- Máy đang xin join hiện fingerprint và `WhoIs`; `y` duyệt, với lựa chọn Member, "grant the same as Machine X" và "also remove Machine X". Duyệt vào Member Admin thì nói rõ là trao quyền Admin.

### 15.5 Named command

- `enter` ở panel Commands chạy lệnh với Active config, **không thoát TUI**: output trong session nhúng (PTY) ở panel chính. Nhiều session cùng lúc, `[`/`]` để chuyển, `x` để đóng.
- Lệnh vi phạm `environments` hoặc thiếu key trong `requires` thì bị chặn kèm lý do.

Nguồn: [Bố cục TUI Bubble Tea](https://github.com/nd2204/kith/issues/9), cùng các mục TUI của [Mô hình ghi và gộp](https://github.com/nd2204/kith/issues/5), [Vòng đời Machine](https://github.com/nd2204/kith/issues/11), [Compaction](https://github.com/nd2204/kith/issues/24), [Rollback](https://github.com/nd2204/kith/issues/25).

## 16. CI và deploy

- Runner CI và nền tảng deploy **không phải Machine** và không cần biết tới kith. Một Machine tin cậy đọc Config rồi **xuất một chiều** vào secret store của nền tảng (CircleCI context, Railway variables).
- Việc xuất là một Named command nối `kith secrets download --format env|json` (stdout) vào CLI của nền tảng (`railway variables --set`, `circleci context store-secret`). Không có connector tích hợp sẵn, không có cờ lọc key (mỗi app/service là một Project).
- Chỉ xuất khi người chạy lệnh; Agent không giữ token nền tảng và không tự đẩy. kith không theo dõi bản đã xuất. Giá trị đã xuất nằm ngoài mô hình thu hồi; đây là đánh đổi được chấp nhận.
- Secret dạng file cho CI (khóa `.p8`) xuất qua base64 trong dòng shell; trên máy chạy kith thì dùng `files:`.
- Máy dùng chung trong tailnet (Mac build, runner tự host) là Machine bình thường, `join` qua Invite, gán vào Member theo vai trò. Mac headless: `kith sync` trước mỗi job, hoặc bật auto-login để Agent chạy.
- Tài liệu có một recipe cho CircleCI và một cho Railway.

Nguồn: [CI và deploy nhận secret thế nào](https://github.com/nd2204/kith/issues/18).

## 17. Cài đặt, phát hành, nâng cấp

### 17.1 Build

- goreleaser trên GitHub Actions khi push tag `v*`. Mỗi release có archive theo OS/arch, `checksums.txt`, `checksums.txt.minisig`, `install.sh`, `install.ps1`.
- `CGO_ENABLED=0` cho `darwin/{amd64,arm64}`, `linux/{amd64,arm64}`, `windows/amd64`. Binary static, chạy cả trên musl/Alpine. Windows arm64 để sau.

### 17.2 Ký

- minisign ký `checksums.txt`. Binary ghim **hai** public key: key đang dùng (private key trong secret của Actions) và key dự phòng (offline, không bao giờ vào CI).
- Lộ hoặc mất key đang dùng: ký bằng key dự phòng một release ghim cặp mới. Mất cả hai thì người dùng cài lại bằng `install.sh`.

### 17.3 Cài

- `curl -fsSL https://github.com/nd2204/kith/releases/latest/download/install.sh | sh`. Script đính kèm từng release. Ghim phiên bản: `sh -s -- --version vX.Y.Z`.
- Cài vào `~/.local/bin/kith`, không sudo; đổi bằng `KITH_INSTALL_DIR`. Thư mục chưa trong `PATH` thì in dòng cần thêm, không sửa file rc.
- `irm …/install.ps1 | iex` cài vào `%LOCALAPPDATA%\Programs\kith`, thêm vào PATH user (HKCU), không cần admin.
- Script bắt buộc kiểm SHA-256; chỉ kiểm chữ ký khi máy có `minisign`, không có thì in hướng dẫn kiểm thủ công.
- Script khởi động lại Agent nếu đã cài; không tự cài Agent (việc của `init`/`join`).

### 17.4 Nâng cấp

- `kith self-update [--version vX]`: tải, **luôn** kiểm chữ ký bằng hai key ghim, thay binary nguyên tử, khởi động lại Agent. Windows: đổi tên exe đang chạy trước khi ghi đè. Binary không nằm ở chỗ `install.sh` cài (ví dụ `go install`) thì từ chối và chỉ cách nâng cấp phù hợp.
- Agent hỏi GitHub API về release `latest` tối đa mỗi 24 giờ, lưu cache cục bộ. TUI hiện ở footer, `doctor` nhắc. Không tự cập nhật, không telemetry. Tắt bằng config hoặc `KITH_NO_UPDATE_CHECK=1`. Lệnh thường không bao giờ chạm internet.

### 17.5 Version và kênh

- SemVer, tag `vX.Y.Z`. Bản thử `-rc.N`, đánh dấu prerelease, `latest` không trỏ vào. Không có nightly.
- `kith version` in version, commit, ngày build.
- Release notes tiếng Anh: tóm tắt viết tay + changelog tự sinh nhóm theo prefix. Mục **Compatibility** bắt buộc khi đổi giao thức hoặc định dạng: ghi minor giao thức mới, `format` mới, có nên chạy `kith projects upgrade` không.

### 17.6 Nền tảng

- macOS và Linux: tier 1. Windows: tier 2 (CI test và build trên `windows-latest`, mọi release có bản Windows, lỗi chỉ có trên Windows không chặn release).
- macOS không codesign, không notarize ở v1 (file tải bằng `curl` không có cờ quarantine).
- `go install` chạy được nhưng không được hỗ trợ chính thức.

Nguồn: [Cài đặt và phát hành kith không cần Go](https://github.com/nd2204/kith/issues/8), [ADR 0007](./adr/0007-ky-release-bang-minisign-hai-khoa-ghim.md).

## 18. `kith doctor`

`doctor` gom mọi kiểm tra sức khỏe; TUI hiện cùng các tín hiệu đó ở nơi tương ứng.

| Mức | Kiểm tra |
|---|---|
| Lỗi | Lệch major giao thức với một peer (đích danh Machine và version nếu từng thấy qua `hello`). |
| Đỏ | Log Project đang đóng băng do Change không đọc được. |
| Cảnh báo | Agent không chạy. |
| Cảnh báo | Change chưa đồng bộ với peer nào sau N giờ. |
| Cảnh báo | Project chỉ có một Machine có quyền Admin. |
| Cảnh báo | Config chỉ có một bản sao (bỏ qua Config đã archive). |
| Cảnh báo | Config đang đóng băng ("viewing an older state, run `kith self-update`"). |
| Cảnh báo | Change không giải mã được (đích danh Machine đã ký); Change bị loại từ máy đã thu hồi. |
| Cảnh báo | Key gắn cờ cần xoay; thu hồi đang chờ xoay khóa. |
| Cảnh báo | Setup hoặc Active config trỏ tới Config đã archive. |
| Cảnh báo | Khóa Machine lưu ở file 0600 vì không có kho khóa. |
| Vàng | Một log vượt 50.000 Change hoặc 50 MB. |
| Thông tin | Peer chạy bản kith cũ hơn; Format floor có thể nâng (gợi ý `kith projects upgrade`); có bản kith mới (gợi ý `self-update`). |

## 19. Bề mặt CLI

Cột "Nguồn" chỉ ticket đã chốt lệnh. Dòng đánh dấu **(đề xuất)** là lệnh PRD bổ sung cho đủ bề mặt, theo UX của Doppler, chưa qua ticket nào; xem mục 22.

| Lệnh | Mục | Nguồn |
|---|---|---|
| `kith` | TUI | #9 |
| `kith init` | 10.1 | #8, #13 (phạm vi: **đề xuất**) |
| `kith setup` | 6.4 | #7 |
| `kith run [name] [-- args]`, `kith run -- <cmd>` | 7 | #6, #7, #9 |
| `kith use <config>`, `kith use --all <config>` | 5.4 | #7, #10 |
| `kith secrets get/set/unset` | 8 | #5 |
| `kith secrets` (liệt kê key) | 8 | **đề xuất** |
| `kith secrets set --from-file [--base64]` | 8 | #26 |
| `kith secrets download`, `kith secrets upload` | 8 | #6, #10 |
| `kith secrets resolve`, `kith conflicts` | 12.5 | #5 |
| `kith secrets flag/unflag` | 13.1 | #24 |
| `kith secrets history/rollback/reapply` | 13 | #25 |
| `kith configs log/rollback` | 13 | #25 |
| `kith configs archive/unarchive`, `kith configs list [--archived]` | 13.4 | #24 |
| `kith configs create` | 22 | **đề xuất** |
| `kith environments create` | 22 | **đề xuất** |
| `kith projects create` | 22 | **đề xuất** |
| `kith projects log`, `kith projects archive/unarchive` | 13 | #24, #25 |
| `kith projects upgrade` | 14.2 | #20 (ticket viết `kith project upgrade`; PRD đổi sang số nhiều cho đồng bộ với các nhóm lệnh khác) |
| `kith invite [--ttl] [--revoke]`, `kith join <mã>` | 10.2 | #13 |
| `kith grants add/revoke <machine> <config>` | 22 | **đề xuất** (TUI đã có, #9) |
| `kith machines remove`, `kith machines create --recovery` | 10 | #11 |
| `kith members remove` | 10.3 | #11 |
| `kith members promote/demote` | 22 | **đề xuất** |
| `kith recover` | 10.4 | #11 |
| `kith backup export/import` | 10.5 | #11 |
| `kith agent`, `kith agent install/uninstall` | 11.1 | #13 |
| `kith sync`, cờ `--sync` | 11.2 | #13 |
| `kith doctor` | 18 | nhiều ticket |
| `kith self-update`, `kith version` | 17 | #8 |
| `kith import vet45 [--dry-run]` | [migration-vet45.md](./migration-vet45.md) | #10 |
| `kith completion` | | #10 |

Biến môi trường: `KITH_CONFIG` (chọn Config), `KITH_INSTALL_DIR` (cài đặt), `KITH_NO_UPDATE_CHECK`. Biến inject cho tiến trình con: `KITH_PROJECT`, `KITH_CONFIG`, `KITH_ENVIRONMENT`, `KITH_MOUNT`.

## 20. Yêu cầu phi chức năng

- **Offline**: `run`, `secrets get/set`, lịch sử, rollback và TUI chạy được khi không có mạng và khi Agent chết.
- **Không lộ secret ra ngoài ý muốn**: không ghi secret vào repo (Mount ngoài repo; Secret file chỉ ghi vào đường dẫn đã gitignore); IPC không mang secret; file secret luôn 0600 / ACL chỉ user; stdout của `run` chỉ dành cho tiến trình con.
- **Bảo mật mạng**: Agent chỉ bind IP tailnet; mọi kết nối mTLS từ khóa Machine; client lạ chỉ chạm endpoint `join` với Invite.
- **Không chạm internet** ngoài kiểm tra bản mới của Agent (tắt được) và `self-update`. Không telemetry.
- **Nền tảng**: macOS, Linux (glibc và musl) tier 1; Windows tier 2. Binary static, không CGO.
- **Quy mô dự kiến**: một Change cỡ 0,5–1 KB; Config đổi 20 lần mỗi tuần thì 5 năm cỡ 5 MB. Ngưỡng xem lại: 50.000 Change hoặc 50 MB mỗi log.
- **Ngôn ngữ**: CLI, TUI, thông báo và release notes tiếng Anh; spec tiếng Việt.

## 21. Hạn chế đã chấp nhận

- Thu hồi không lấy lại được giá trị máy đã giữ; kith chỉ xoay data key và gắn cờ.
- Giá trị đã xuất sang CI/deploy nằm ngoài mô hình thu hồi.
- Mọi giá trị từng ghi nằm vĩnh viễn trong lịch sử; không có purge.
- Không audit được việc đọc.
- Không có relay: các máy không cùng online thì nhân bản bị trễ.
- Máy chạy bản cũ có thể đóng băng Config và chưa thấy lệnh thu hồi mới cho tới khi nâng cấp.
- Setup viết cho Unix không chạy trên Windows.
- Không có lịch sao lưu tự động.

## 22. Điểm để ngỏ cho implement

Các ticket đã chốt mọi quyết định khó đảo ngược. Khi gom lại thành PRD, còn vài chỗ chưa ticket nào nói tới. Chúng dễ đảo ngược nên PRD ghi đề xuất mặc định; người implement xác nhận hoặc sửa khi làm tới.

**Bề mặt lệnh quản trị** (chỉ có trong TUI, hoặc chưa được nhắc tới):

1. **Tạo Project, Environment, Config, Branch config.** Đề xuất theo Doppler: `kith projects create <slug>`, `kith environments create <slug>` (tạo luôn root Config cùng tên), `kith configs create <name> --branch-of <root>`. Đều là Change trong log Project do Admin ký. Máy tạo Config sinh data key epoch đầu và tự có Grant. Máy tạo Project trở thành Machine của Member Admin đầu tiên.
2. **Phạm vi của `kith init`.** Đề xuất: sinh khóa Machine, đặt tên Member cho máy, hỏi cài Agent. Không tạo Project (việc của `projects create`).
3. **Grant trên CLI.** Đề xuất: `kith grants add|revoke <machine> <config>` và `kith grants list`, tương đương thao tác `space` trong panel Machines.
4. **Phong và bỏ Admin.** Đề xuất: `kith members promote|demote <member>`, Change do Admin ký; không demote được Admin cuối cùng.
5. **Liệt kê key**: `kith secrets` không kèm lệnh con in danh sách key (che giá trị), như Doppler.
6. **Đổi tên** Environment, Config hay Member: chưa có. Đề xuất không làm ở v1 (Project đã là slug không đổi được).

**Tham số implement tự chọn** (đã chốt là có, chưa chốt giá trị): cổng mặc định của Agent; timeout của `--sync`; số giờ N cho cảnh báo chưa đồng bộ; giới hạn yêu cầu join đang chờ mỗi Invite; đường dẫn kho cục bộ và thư mục trạng thái theo từng hệ điều hành; mã hóa nhị phân của header và TLV; định dạng mã Invite; chi tiết HLC.

## 23. Nguồn

**Ticket** (comment kết luận là nguồn của quyết định):

- [Chọn lớp mạng riêng cho kith](https://github.com/nd2204/kith/issues/2) · [research/network-layer.md](https://github.com/nd2204/kith/blob/research/network-layer/research/network-layer.md)
- [Khảo sát mô hình Doppler và các tool secret local-first](https://github.com/nd2204/kith/issues/3) · [research/doppler-model.md](https://github.com/nd2204/kith/blob/research/doppler-model/research/doppler-model.md)
- [Mô hình cấp, thu hồi quyền Config và mã hóa end-to-end](https://github.com/nd2204/kith/issues/4)
- [Mô hình ghi và gộp xung đột khi nhân bản secret](https://github.com/nd2204/kith/issues/5)
- [Inject secret cho công cụ cần file (Flutter dart-define-from-file)](https://github.com/nd2204/kith/issues/6)
- [Định dạng file Setup và Named command](https://github.com/nd2204/kith/issues/7)
- [Cài đặt và phát hành kith không cần Go](https://github.com/nd2204/kith/issues/8)
- [Bố cục TUI Bubble Tea của kith](https://github.com/nd2204/kith/issues/9)
- [Kế hoạch chuyển từ vet45_env sang kith](https://github.com/nd2204/kith/issues/10)
- [Vòng đời Machine: gỡ máy, mất máy và khôi phục khi mất hết Admin](https://github.com/nd2204/kith/issues/11)
- [Tiến trình đồng bộ: daemon hay theo lệnh](https://github.com/nd2204/kith/issues/13)
- [Ràng buộc tác giả vào Change khi mã hóa trước rồi ký](https://github.com/nd2204/kith/issues/14)
- [CI và deploy nhận secret thế nào](https://github.com/nd2204/kith/issues/18)
- [Tương thích phiên bản giao thức và định dạng Change giữa các Machine](https://github.com/nd2204/kith/issues/20)
- [Compaction và giữ lịch sử log Change](https://github.com/nd2204/kith/issues/24)
- [Xem lịch sử và rollback secret](https://github.com/nd2204/kith/issues/25)
- [Secret dạng file ghi ra đường dẫn cố định](https://github.com/nd2204/kith/issues/26)

**ADR**: [0001](./adr/0001-machine-la-recipient-quyen-trong-du-lieu-kith.md) Machine là recipient · [0002](./adr/0002-age-x25519-va-ed25519.md) age X25519 và Ed25519 · [0003](./adr/0003-log-change-co-chu-ky-dang-dag.md) Log Change dạng DAG · [0004](./adr/0004-moi-app-mot-project-setup-khong-bien-doi-key.md) Mỗi app một Project · [0005](./adr/0005-khoi-phuc-admin-bang-recovery-machine.md) Recovery machine · [0006](./adr/0006-dong-bo-qua-agent-cap-user-va-mtls.md) Agent và mTLS · [0007](./adr/0007-ky-release-bang-minisign-hai-khoa-ghim.md) minisign hai khóa · [0008](./adr/0008-tuong-thich-phien-ban-bang-khung-header-co-dinh-va-format-floor.md) Format floor · [0009](./adr/0009-giu-lich-su-vinh-vien-archive-thay-cho-xoa.md) Giữ lịch sử vĩnh viễn

**Prototype**: [`prototype/tui-layout`](https://github.com/nd2204/kith/tree/prototype/tui-layout/prototypes/tui-layout) (không merge).
