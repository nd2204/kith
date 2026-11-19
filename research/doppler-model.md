# Khảo sát mô hình Doppler và các tool secret local-first cùng loại

Ticket: [#3](https://github.com/nd2204/kith/issues/3). Ngày tra cứu: 2026-11-19. Mã nguồn Doppler CLI đọc ở nhánh `master`, bản phát hành mới nhất lúc tra là `3.77.1`.

## Câu hỏi

kith học UX của Doppler. Những khái niệm và hành vi nào của Doppler và các tool cùng loại nên mang sang, và chúng vận hành cụ thể ra sao? Với các tool phân tán hoặc local-first: chúng mã hóa, chia sẻ khóa và gộp thay đổi như thế nào? Kết quả cuối là bảng "mang sang / bỏ / sửa lại" ở [mục 4](#4-bảng-mang-sang--bỏ--sửa-lại), dùng làm đầu vào cho các ticket grilling #4–#7.

## Tóm tắt

- Mô hình dữ liệu của Doppler chuyển sang kith gần như nguyên vẹn: Project → Environment → root Config → Branch config một tầng. Branch config phủ lên root theo từng key. Đây đúng là vocabulary trong `GLOSSARY.md`.
- Doppler resolve tham chiếu `${KEY}` / `${config.KEY}` / `${project.config.KEY}` **lúc đọc**. Tham chiếu không resolve được thì để nguyên chuỗi `${...}`. Trong mô hình phân tán, việc resolve chỉ làm được trên máy có replica của Config đích, nên tham chiếu chéo Config là chỗ chạm vào mô hình cấp quyền (#4).
- File `doppler.yaml` commit trong repo **không** được `doppler run` đọc. Chỉ `doppler setup` đọc nó, rồi ghi project/config vào `~/.doppler/.doppler.yaml` theo đường dẫn thư mục. Setup của kith có thể làm đơn giản hơn: `kith run` đọc thẳng file commit.
- `doppler run --mount` không ghi file tạm thường. Nó tạo **named pipe (FIFO) quyền 0600**, không chạy trên Windows, và khi dùng thì secret **không** được inject vào env. `sops exec-file` cũng mặc định dùng FIFO và có `--no-fifo` để rơi về file tạm. Đây là đầu vào trực tiếp cho #6.
- Doppler đã có sẵn chế độ đọc offline: mỗi lần `run` thành công, nó ghi một "fallback file" mã hóa AES-256-GCM, khóa sinh từ PBKDF2. Replica cục bộ của kith là phiên bản tổng quát của cơ chế này.
- Trong các tool đã xem, cách mã hóa "mỗi value một ciphertext, key để rõ" của SOPS và dotenvx cho diff có nghĩa. Nhưng SOPS vẫn xung đột git vì MAC và `lastmodified` toàn file đổi ở mỗi lần sửa ([sops#52](https://github.com/getsops/sops/issues/52), [sops#1117](https://github.com/getsops/sops/issues/1117)). Gộp tự động cần metadata theo từng key, không theo từng file.
- Thu hồi quyền chỉ có ý nghĩa khi đi kèm xoay khóa. SOPS khuyên `rotate` khi bỏ recipient. git-crypt không thu hồi được. Các tool P2P (env.sync.local, sigyn) đều mã hóa lại cho tập recipient mới và phát một sự kiện thu hồi có chữ ký.
- Chưa tool P2P nào đã xem có gộp xung đột theo key thật sự chạy được. env.sync.local so timestamp rồi "merge". sigyn mô tả vector clock + LWW-Map nhưng tự ghi trong docs là "automated secret-level conflict resolution is not yet implemented". kith cần tự chốt mô hình này ở #5.

## 1. Doppler

### 1.1 Project, Environment, Config

- Mỗi Project có một bộ Environment mặc định (dev, stg, prd) và có thể tùy biến, ví dụ thêm `ci`. Mỗi Environment có một **root config** chứa "master list of secrets for that environment". Sửa root thì các branch config bên dưới cũng đổi theo. ([Configs](https://docs.doppler.com/docs/root-configs.md))
- Dashboard cảnh báo **missing secret** khi một key có ở Environment này mà thiếu ở Environment khác, và cho phép dismiss. ([Configs](https://docs.doppler.com/docs/root-configs.md#missing-secret-detection))
- Khi lưu, có thể **cascade** thay đổi sang Environment khác theo kiểu PATCH: chỉ thuộc tính đã đổi (name, value, visibility) được áp sang. ([Secrets → Cascading Changes](https://docs.doppler.com/docs/secrets.md))
- Biến dành riêng được inject kèm: `DOPPLER_PROJECT`, `DOPPLER_ENVIRONMENT`, `DOPPLER_CONFIG`. ([Secrets → Reserved Secrets](https://docs.doppler.com/docs/secrets.md))

### 1.2 Branch config và kế thừa

- Tên branch config luôn có tiền tố là tên Environment, ví dụ `dev_stripe_billing`. Branch tự nhận mọi thay đổi của root, còn giá trị riêng của branch được "overlaid" lên trên. Nhân bản (duplicate) một branch tạo ra branch mới **gắn vào cùng root**, giữ nguyên phần overlay. ([Branch Configs](https://docs.doppler.com/docs/branch-configs.md))
- Tài liệu không nói thẳng rằng chỉ có một tầng. Nhưng mọi thao tác đều mô tả quan hệ branch → root, và duplicate cũng không tạo branch-của-branch. Kết luận "một tầng" là suy luận từ cách tài liệu mô tả.
- Quy trình "promote": thêm key vào các root config, sau đó xóa branch. ([Branch Configs → Sharing a Config](https://docs.doppler.com/docs/branch-configs.md))
- **Personal config** (`dev_personal`) là một branch riêng tư cho mỗi user có quyền ghi Environment đó. Tính năng này bật mặc định cho `dev`, và có thể nhắm tới qua `doppler.yaml` (`config: dev_personal`). ([Branch Configs → Personal Configs](https://docs.doppler.com/docs/branch-configs.md))
- **Config Inheritance** khác với branch config. Đây là kế thừa chéo Project, chỉ có ở gói trả phí. Config cha phải bật "inheritable". Khi nhiều cha trùng key thì cha xếp trên cùng thắng, còn key của chính config con luôn thắng. ([Config Inheritance](https://docs.doppler.com/docs/config-inheritance.md))
- Có thể **lock** một config để chặn đổi tên hoặc xóa. ([Branch Configs → Locking](https://docs.doppler.com/docs/branch-configs.md))

### 1.3 Tham chiếu secret

| Kiểu | Cú pháp |
| :-- | :-- |
| Cùng config | `${SECRET_NAME}` |
| Khác config, cùng project | `${config.SECRET_NAME}` |
| Khác project | `${project.config.SECRET_NAME}` |

([Secrets → Referencing Secrets / Across Projects](https://docs.doppler.com/docs/secrets.md))

- **Resolve lúc đọc.** Nếu đích không còn (bị đổi tên hoặc xóa), Doppler để nguyên chuỗi `${...}` làm giá trị. Khi đích xuất hiện trở lại, giá trị "immediately start injecting the reference". Tài liệu tự cảnh báo đây là rủi ro.
- Tham chiếu chéo config/project chỉ có ở gói trả phí. User chỉ tạo được tham chiếu tới secret mà mình có quyền đọc, nhưng các tham chiếu đã tồn tại thì vẫn còn đó.
- `doppler secrets --raw` hiện cả giá trị đã resolve và giá trị thô.
- Tên secret chứa `.` thì phải viết `${"stripe.api_key"}`.
- Có thể tắt tính năng tham chiếu ở cấp workplace, project hoặc config.
- Infisical dùng cùng cú pháp `${KEY}` / `${env.KEY}` / `${env.folder.KEY}`, và ghi rõ là "reconstructs values on the client side". Vì vậy client phải có quyền đọc mọi scope liên quan, thiếu quyền thì tham chiếu không resolve. ([Infisical: Secret referencing](https://infisical.com/docs/documentation/platform/secret-reference.md))

### 1.4 `doppler setup` và `doppler.yaml`

- Định dạng file ([`pkg/models/repo_config.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/models/repo_config.go)): khóa `setup` là một object `{project, config, path}`, hoặc một list các object đó cho monorepo, kèm khóa `flags`. Khi có nhiều entry thì mỗi entry phải có `path` và các `path` không được trùng nhau ([`pkg/cmd/setup.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/cmd/setup.go)).
- **Ai đọc file này:** chỉ `doppler setup`. Lệnh này hỏi "Use settings from repo config file (doppler.yaml)?", sau đó gọi `configuration.Set(expandedPath, ...)`, tức là ghi vào `~/.doppler/.doppler.yaml` với khóa là đường dẫn thư mục ([setup.go](https://github.com/DopplerHQ/cli/blob/master/pkg/cmd/setup.go)). Khi chạy `doppler run`, CLI tra project/config theo thứ tự: token (`DOPPLER_TOKEN`/`--token`) > flag `-p`/`-c` > entry khớp đúng thư mục trong `~/.doppler/.doppler.yaml` > entry của thư mục cha gần nhất. Doppler tự ghi nhận hệ quả: "if you move a project directory, it will lose its Doppler configuration". ([Secrets Setup Guide → Under the Hood](https://docs.doppler.com/docs/secrets-setup-guide.md))
- `doppler setup --no-interactive` dùng cho CI và script. ([CLI Guide](https://docs.doppler.com/docs/cli.md))
- Đối chiếu: Infisical có `.infisical.yaml` commit được, do `infisical init` tạo. CLI **đọc trực tiếp** file này lúc chạy, và file có thể ánh xạ git branch sang environment (`overrides.git-branch`). ([Infisical: CLI project configuration](https://infisical.com/docs/cli/project-config.md)) Sigyn có `.sigyn.toml` gồm `[project]` (vault, env, identity) và `[commands]`. Mục `[commands]` chính là khái niệm *Named command* của kith (`sigyn run dev` chạy `npm run dev` với secret đã inject). ([sigyn README](https://github.com/tonybenoy/sigyn#project-config-sigyntoml))

### 1.5 `doppler run`

Nguồn: [`pkg/cmd/run.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/cmd/run.go), [`pkg/controllers/secrets.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/controllers/secrets.go).

- Hai dạng gọi là `doppler run -- CMD ARGS` và `doppler run --command "a && b"`; hai dạng này loại trừ nhau.
- `--preserve-env=FOO,BAR` (hoặc `true`) cho biến env đã có sẵn thắng giá trị secret. Mặc định là secret thắng.
- `--only-secrets` chỉ inject một tập con. `--name-transformer` đổi tên biến, ví dụ `dotnet-env`, `tf-var`.
- `--watch` (BETA) khởi động lại process khi secret thay đổi. `--forward-signals` chuyển tiếp signal cho process con.
- CLI có một danh sách tên biến nguy hiểm được kiểm tra trước khi inject (`LD_PRELOAD`, `DYLD_INSERT_LIBRARIES`, `PROMPT_COMMAND`, `PERL5OPT`, …), xem `dangerousSecretNames` trong controllers/secrets.go.
- **`--mount <path>`** ghi secret ra "an ephemeral file, accessible at DOPPLER_CLI_SECRETS_PATH. when enabled, secrets are NOT injected into the environment". Cách cài đặt:
  - Tạo **named pipe** (`CreateNamedPipe(mountPath, 0o600)`). Nếu đường dẫn đã tồn tại thì báo lỗi. Pipe bị xóa khi process kết thúc.
  - Một goroutine mở, ghi rồi đóng pipe lặp lại, nên mỗi lần process con đọc là nhận lại toàn bộ nội dung. `--mount-max-reads N` giới hạn số lần đọc, mặc định 0 là không giới hạn.
  - Trên Windows `SupportsNamedPipes = false` ([`pkg/utils/io_windows.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/utils/io_windows.go)), nên `--mount` báo lỗi "This OS does not support mounting a secrets file".
  - Định dạng: `--format` (`--mount-format` là alias cũ) nhận `env`, `json`, `dotnet-json`, `template`, `env-no-quotes`, `docker`. Nếu không chỉ định, CLI đoán theo đuôi file (`.env` → env, `.json` → json, file `appsettings*.json` → dotnet-json). `--mount-template <file>` render secret qua Go `text/template` ([`pkg/models/secrets_format.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/models/secrets_format.go)).
- **Fallback file (đọc offline).** Mỗi lần fetch thành công, CLI ghi một snapshot JSON **mã hóa** vào thư mục cấu hình của Doppler. Mặc định passphrase được tính từ token + project + config, có thể đổi bằng `--passphrase`. Thuật toán là PBKDF2 + AES-256-GCM. Các flag liên quan: `--fallback-only`/`--offline` chỉ đọc từ file, `--fallback-readonly` không ghi file, `--no-fallback` tắt hẳn. Tài liệu ghi file này "can be used indefinitely" và chỉ hỗ trợ định dạng JSON. ([Secret Fallback Files](https://docs.doppler.com/docs/automatic-fallbacks.md); số vòng PBKDF2 nằm trong [`pkg/models/encrypted_file.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/models/encrypted_file.go))

### 1.6 `doppler secrets download` / `substitute`

- `doppler secrets download [path]` mặc định ghi một file **mã hóa**, dạng JSON. `--format` nhận `json`, `dotnet-json`, `env`, `yaml`, `docker`, `env-no-quotes`. Chỉ khi có `--no-file` thì lệnh mới in plaintext ra stdout. ([`pkg/cmd/secrets.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/cmd/secrets.go))
- `doppler secrets substitute <template>` render một template Go có chứa secret.
- `doppler secrets upload <file>` nhận ENV, JSON hoặc YAML. Có thể chép giữa hai config bằng `upload <(download --no-file)`. ([Secrets → Importing](https://docs.doppler.com/docs/secrets.md))

### 1.7 Version, rollback, log

- Mọi hành động trên config được ghi log "similar to how `git log` works". Các thay đổi rollback được là thêm/sửa/xóa secret và Trusted IP. Rollback tạo **một log entry mới** đảo ngược thay đổi cũ, không xóa lịch sử. ([Versioning](https://docs.doppler.com/docs/versioning.md))
- Mỗi version của một secret được giữ suốt vòng đời của secret đó. **Redact** làm một version cũ không truy cập được nữa nhưng "the value itself is still stored on Doppler's end". Không redact được version hiện hành. ([Versioning → Retention](https://docs.doppler.com/docs/versioning.md), [Secrets → Redaction](https://docs.doppler.com/docs/secrets.md))
- Có hai loại log: Activity log cấp workplace (`doppler activity`) và config log (`doppler configs logs`, `logs get`, `logs rollback`). ([Activity Logs](https://docs.doppler.com/docs/workplace-logs.md), [`pkg/cmd/configs_logs.go`](https://github.com/DopplerHQ/cli/blob/master/pkg/cmd/configs_logs.go))
- Infisical làm tương tự: secret versioning, cộng thêm "commit" theo environment/folder để khôi phục point-in-time. ([Secret versioning](https://infisical.com/docs/documentation/platform/secret-versioning.md), [Point-in-time recovery](https://infisical.com/docs/documentation/platform/pit-recovery.md))

### 1.8 Lệnh CLI chính

Danh sách lấy từ `pkg/cmd/` ([cây thư mục](https://github.com/DopplerHQ/cli/tree/master/pkg/cmd)) và [CLI Guide](https://docs.doppler.com/docs/cli.md): `login`, `logout`, `me`, `setup`, `configure` (`get/set/unset/reset/debug`, có `--scope`), `run` (cùng `run clean`), `secrets` (`get/set/upload/delete/download/substitute`, `notes`), `projects`, `environments`, `configs` (`create/delete/clone/lock/unlock`, `logs`, `tokens`), `activity`, `import`, `open`, `tui` (BETA), `update`, `completion`. Các flag toàn cục đáng chú ý: `--token`, `--json`, `--print-config`, `--no-read-env`, `--timeout`.

## 2. Các tool cùng loại

### 2.1 SOPS + age

Nguồn: [SOPS reference → Encryption Protocol](https://github.com/getsops/docs/blob/main/content/en/docs/reference/_index.md), [SOPS security](https://github.com/getsops/docs/blob/main/content/en/docs/security/_index.md), [key management](https://github.com/getsops/docs/blob/main/content/en/docs/usage/key-management/_index.md), [advanced usage](https://github.com/getsops/docs/blob/main/content/en/docs/usage/advanced/_index.md), [age spec (C2SP)](https://github.com/C2SP/C2SP/blob/main/age.md).

- **Mã hóa.** Mỗi file có một data key 256-bit ngẫu nhiên. Data key được mã hóa riêng cho từng master key (age, PGP, KMS…) và lưu trong khối `sops:`. Mỗi *leaf value* được mã hóa AES256-GCM bằng data key, với IV riêng. **Tên key để rõ**, và chuỗi key-path được dùng làm AAD. Một MAC tính trên toàn bộ value (cũng được mã hóa bằng data key) chống thêm hoặc bớt value.
- **Chia sẻ khóa.** Có nhiều recipient. *Key group* + `shamir_threshold` cho phép yêu cầu M-of-N group mới giải được. `.sops.yaml` có `creation_rules` để chọn recipient theo đường dẫn file.
- **Thu hồi.** `sops updatekeys` thêm hoặc bớt recipient nhưng giữ nguyên data key. Khi bỏ recipient, tài liệu khuyên dùng `rotate` (sinh data key mới) "otherwise, owners of the removed key may have add access to the data key in the past" (nguyên văn, kể cả lỗi chính tả).
- **Gộp.** Mục tiêu thiết kế ban đầu là "Encrypting each entry separately… conflicts are easier to resolve… as long as they don't modify the same values". Thực tế, MAC và `lastmodified` toàn file đổi ở **mọi** lần sửa, nên hai nhánh sửa hai key khác nhau vẫn xung đột ở khối `sops:`. Các issue mở lâu năm: [#52](https://github.com/getsops/sops/issues/52), [#1117](https://github.com/getsops/sops/issues/1117), [#1712](https://github.com/getsops/sops/issues/1712).
- **Chạy.** `sops exec-env file 'cmd'` inject secret vào env. `sops exec-file file 'cmd {}'` mặc định dùng **FIFO**, theo docs: "plaintext secrets never touch the disk, and the child process can only read the secrets once". Có `--no-fifo` để dùng file tạm thường (cho Windows, hoặc khi process cần đọc nhiều lần), file tạm bị xóa khi process kết thúc.
- **age.** File key 16 byte. Mỗi recipient có một *stanza* bọc cùng file key đó (X25519, scrypt, hoặc ML-KEM768-X25519 hậu lượng tử). Header được MAC bằng HMAC-SHA-256 với khóa dẫn từ file key. Payload dùng ChaCha20-Poly1305 theo chunk 64 KiB. Stanza scrypt phải đứng một mình trong header. CLI chấp nhận SSH public key (`ssh-ed25519`, `ssh-rsa`) làm recipient, và có plugin cho YubiKey ([age README](https://github.com/FiloSottile/age)). Suy ra từ spec: người gửi tự sinh file key, nên ai có public key của recipient cũng tạo được một file hợp lệ. **age không xác thực người gửi**, nên kith cần chữ ký riêng (ví dụ Ed25519) nếu muốn biết thay đổi đến từ máy nào.

### 2.2 Infisical (self-host)

- Kiến trúc client–server: API server + PostgreSQL. Secret được mã hóa AES-256-GCM **ở phía server**, với cây khóa phân tầng: root key (biến môi trường `ENCRYPTION_KEY`, hoặc HSM/KMS ngoài) → internal KMS → khóa từng project. Không phải E2EE: server thấy plaintext khi xử lý. ([Infisical Security](https://infisical.com/docs/internals/security.md))
- Khái niệm có thể học: **personal overrides**, docs ví von "a shared value is to a `main` branch as a personal value is to a custom branch" ([Project](https://infisical.com/docs/documentation/platform/secrets-mgmt/project.md)). Ngoài ra có folder, secret import, tham chiếu `${env.folder.KEY}`, và `.infisical.yaml` ánh xạ git branch sang environment.
- Đọc offline: CLI ghi một bản sao mã hóa (`secrets-backup/`, quyền 0600) sau mỗi lần fetch, và chỉ dùng nó khi mất mạng ([`packages/util/secrets.go`](https://github.com/Infisical/cli/blob/main/packages/util/secrets.go)). Cách làm giống fallback của Doppler.
- Gộp: không áp dụng, vì server là nguồn sự thật duy nhất.

### 2.3 Teller

- Teller không lưu trữ gì. Nó là một lớp "universal" kéo secret từ các provider (Vault, AWS SM, GCP, dotenv, …) theo `.teller.yml`, có template `{{ get_env(...) }}` trong đường dẫn. ([Teller README](https://github.com/tellerops/teller))
- Các tính năng đáng học: `teller run --reset --shell -- cmd`, `teller env` / `teller export json|yaml`, `teller copy --from a --to b` để đồng bộ giữa provider, `teller scan` (dò secret bị lộ trong code, có `--error-if-found` cho CI), và `teller redact` (che giá trị secret trong log, stdin hoặc file).
- Không có mã hóa, chia sẻ hay gộp riêng: tất cả giao cho provider.

### 2.4 dotenvx

- File `.env` được **commit dưới dạng đã mã hóa từng value**. Mỗi file có một cặp khóa secp256k1: `DOTENV_PUBLIC_KEY` nằm trong file, `DOTENV_PRIVATE_KEY[_SUFFIX]` nằm trong `.env.keys` hoặc trong secret manager. Mỗi value được mã hóa ECIES, tức là khóa AES-256 ephemeral được bọc bằng public key. ([dotenvx README, FAQ "How does encryption work?"](https://github.com/dotenvx/dotenvx))
- Chia sẻ khóa = chia sẻ private key của file. Không có danh sách recipient theo từng người, nên muốn thu hồi thì phải sinh khóa mới và chuyển lại khóa đó cho những người còn quyền.
- Có các hành vi UX đáng học: `dotenvx run -f a,b` (file trước thắng, `--overload` để đảo thứ tự), suffix tên khóa chọn file (`DOTENV_PRIVATE_KEY_PRODUCTION` → `.env.production`), mở rộng `${VAR}` và `$(cmd)` trong value.
- Gộp: dùng git. Vì key để rõ và mỗi value là một dòng ciphertext, diff theo dòng hoạt động khi hai người sửa hai key khác nhau.

### 2.5 git-crypt

- Mã hóa trong suốt qua git filter (`.gitattributes`). Thuật toán AES-256-CTR với IV tổng hợp từ HMAC-SHA-1 của file, nên mã hóa *tất định*: hai file giống nhau cho cùng ciphertext. Khóa đối xứng được chia sẻ bằng GPG (`add-gpg-user` commit khóa đã mã hóa vào `.git-crypt/`). ([git-crypt README](https://github.com/AGWA/git-crypt))
- Giới hạn được tác giả ghi rõ: không mã hóa tên file hay metadata; **không thu hồi được** (không có `del-gpg-user`, không xoay khóa), vì "a user having the previous key can still access previous repository history"; ciphertext không nén được; kẻ sửa được repo có thể tắt mã hóa bằng cách sửa `.gitattributes`.
- Gộp: cả file là một khối nhị phân, nên mọi xung đột phải giải bằng tay sau khi giải mã.

### 2.6 gopass (git + GPG/age, nhiều recipient)

- Mỗi secret là một file mã hóa riêng (GPG hoặc age), và mỗi store hay mount có danh sách recipient riêng (`gopass recipients add/remove --store`). `gopass fsck` kiểm tra và mã hóa lại các file còn thiếu recipient. Đồng bộ bằng `gopass sync` qua git remote. Backend age chấp nhận cả SSH key (`ssh-ed25519`, `ssh-rsa`). ([gopass features](https://github.com/gopasspw/gopass/blob/master/docs/features.md), [age backend](https://github.com/gopasspw/gopass/blob/master/docs/backends/age.md))
- Gộp: mỗi secret một file, nên hai người sửa hai secret khác nhau không xung đột. Sửa cùng một secret thì xung đột trên một blob nhị phân.

### 2.7 Tool nhân bản peer-to-peer

Tìm được hai tool đúng mô hình "không server". Cả hai còn nhỏ: env.sync.local có ~40 star, sigyn ~7 star và tạo từ 2026-02. Nên coi đây là tham khảo thiết kế, chưa phải bằng chứng đã kiểm chứng trong thực tế.

**env.sync.local** ([README](https://github.com/championswimmer/env.sync.local), [SECURITY-MODES](https://github.com/championswimmer/env.sync.local/blob/main/docs/SECURITY-MODES.md))

- Phát hiện peer qua mDNS trên LAN. Có ba chế độ tin cậy. Ở chế độ `secure-peer`: secret được mã hóa age at-rest, truyền qua HTTPS + mTLS, peer mới vào bằng *invite token* có hạn rồi được một peer cũ *approve*. Peer ID có dạng `user@host`.
- **Thành viên được lan truyền dưới dạng sự kiện có chữ ký** (`peer_joined`, `secrets_reencrypted`, revoke). Peer nhận sự kiện kiểm tra chữ ký, kiểm tra "Event ID > last applied" và kiểm tra hạn timestamp. Peer offline khi quay lại thì hỏi các sự kiện từ cursor cuối cùng. Thu hồi một peer nghĩa là mã hóa lại không có age key của peer đó và phát sự kiện thu hồi.
- Gộp: so timestamp, nếu bên kia mới hơn thì giải mã, "merge changes", rồi mã hóa lại cho mọi recipient. Docs không nói rõ gộp theo key hay thay cả file. Mỗi lần ghi đè, tool sao lưu lại 5 bản gần nhất.

**sigyn** ([README](https://github.com/tonybenoy/sigyn), [sync.md](https://github.com/tonybenoy/sigyn/blob/main/docs-site/src/sync.md), [architecture.md](https://github.com/tonybenoy/sigyn/blob/main/docs-site/src/architecture.md))

- Tự giới thiệu là "A Doppler alternative". Có vault → env, `.sigyn.toml` với `[commands]`, và chạy được ở chế độ `sigyn run serve`, tức là cấp secret qua Unix socket 0600 thay vì qua env.
- Mã hóa: envelope X25519. `members.cbor` giữ slot khóa vault và slot khóa cho từng env, dữ liệu mã hóa ChaCha20-Poly1305, passphrase dẫn khóa bằng Argon2id. Audit log được hash-chain và ký Ed25519. Transport là git (remote chỉ thấy ciphertext).
- Thu hồi: xóa slot của thành viên và **xoay master key**. Thay đổi lan qua lần push/pull kế tiếp.
- Gộp: mô tả mỗi entry gồm `{value, timestamp, vector clock, writer}`. LWW-Map lấy timestamp lớn hơn; vector clock phát hiện ghi đồng thời; có 6 chiến lược giải (TakeLocal/Remote/LatestTimestamp/HigherRole/Merge/Defer). Nhưng docs ghi rõ: "Automated secret-level conflict resolution is **not yet implemented**". Khi lịch sử phân nhánh, người dùng phải tự `git merge` trong thư mục vault.

### 2.8 So sánh cách mã hóa, chia sẻ khóa, gộp

| Tool | Đơn vị mã hóa | Chia sẻ khóa | Thu hồi | Gộp thay đổi đồng thời |
| :-- | :-- | :-- | :-- | :-- |
| Doppler | Server-side, user không thấy | Quyền trên server | Bỏ quyền trên server | Server tuần tự hóa |
| Infisical | Server-side AES-GCM, khóa phân tầng | Quyền trên server | Bỏ quyền trên server | Server tuần tự hóa |
| SOPS | Từng value, một data key mỗi file | Nhiều recipient + Shamir | `updatekeys` + `rotate` | Git; xung đột ở MAC/`lastmodified` |
| dotenvx | Từng value (ECIES) | Một private key mỗi file | Sinh khóa mới, chia lại | Git, theo dòng |
| git-crypt | Cả file, tất định | GPG bọc khóa đối xứng | Không hỗ trợ | Git, blob nhị phân |
| gopass | Từng secret (file) | Recipient theo store | `recipients remove` + mã hóa lại | Git, theo file |
| env.sync.local | Cả file `.secrets.env` (age) | Invite + approve, sự kiện có chữ ký | Mã hóa lại + sự kiện thu hồi | So timestamp |
| sigyn | Envelope theo vault/env | Slot X25519 cho từng thành viên | Xóa slot + xoay master key | Thiết kế LWW + vector clock, hiện tự merge bằng git |

## 3. Hệ quả cho mô hình phân tán của kith

- **Đơn vị nhân bản = Config.** Doppler đã coi Config là đơn vị đọc và inject. Branch config chỉ chứa phần overlay, nên muốn đọc được thì máy phải có **cả branch lẫn root** của nó. Cấp quyền một branch config vì thế ngầm kéo theo quyền đọc root (#4).
- **Tham chiếu chéo Config.** Doppler và Infisical đều chỉ resolve được tham chiếu khi người đọc có quyền tới đích. Trong kith, điều này có nghĩa là máy phải có replica của Config đích, không thì chuỗi `${...}` sẽ bị để nguyên. Hành vi "để nguyên rồi tự điền khi đích xuất hiện" là rủi ro mà chính Doppler đã cảnh báo. kith nên chọn: báo lỗi khi resolve thất bại, hoặc ít nhất là cảnh báo.
- **Đọc offline** là trạng thái bình thường của kith, không phải fallback. Mỗi máy có replica được mã hóa at-rest. Nhờ đó, các flag `--fallback*` của Doppler không cần trong kith.
- **Gộp**: các tool dựa trên git cho thấy gộp theo file hay theo dòng không đủ. Sigyn chỉ ra hình dạng metadata cần có (giá trị + clock + writer theo từng key, cộng tombstone khi xóa), nhưng chưa có cài đặt chạy thật. Đây là câu hỏi trung tâm của #5.
- **Thu hồi**: mọi tool nghiêm túc đều xoay khóa và mã hóa lại khi bỏ một thành viên. Không tool nào lấy lại được dữ liệu cũ mà máy bị thu hồi đã giữ, và git-crypt nói thẳng điều này. kith nên ghi rõ giới hạn đó trong spec: sau khi bị thu hồi, máy đó không nhận được *giá trị mới*, còn giá trị cũ phải được xoay ở phía nhà cung cấp.
- **File cho tool cần file (#6)**: cả Doppler và SOPS đều chọn FIFO quyền 0600 làm mặc định, file tạm thường là lựa chọn phụ. Còn mở: các tool như `flutter --dart-define-from-file` có đọc file nhiều lần hoặc `stat` kích thước file không. Nếu có, FIFO chỉ cho đọc một lần sẽ hỏng, và kith cần chế độ file tạm 0600 xóa khi process kết thúc, tương đương `--no-fifo`. Cần thử nghiệm ở #6.

## 4. Bảng mang sang / bỏ / sửa lại

| Khái niệm (nguồn) | Quyết định | Lý do |
| :-- | :-- | :-- |
| Project → Environment → root Config (Doppler) | **Mang sang** | Đúng với `GLOSSARY.md`. Config là đơn vị cấp quyền và nhân bản. |
| Branch config một tầng, phủ theo key, tên có tiền tố env (Doppler) | **Mang sang** | Mô hình đơn giản, dễ giải thích. Một tầng tránh chuỗi kế thừa dài khi replica thiếu một mắt xích. |
| Branch phụ thuộc root khi đọc (Doppler) | **Sửa lại** | Phải nhân bản root cùng branch, và cấp quyền branch thì ngầm cấp quyền đọc root. Cần chốt ở #4. |
| Personal config `dev_personal` (Doppler) / personal overrides (Infisical) | **Sửa lại** | Không có "user trên server". Có thể là một Branch config chỉ nhân bản tới các máy của một người. Cần quyết ở #4. |
| Config Inheritance chéo Project, nhiều cha, có thứ tự (Doppler) | **Bỏ** (v1) | Thêm một trục phụ thuộc replica và quyền. Tham chiếu `${project.config.KEY}` đã đủ cho các ca dùng chung. |
| Tham chiếu `${KEY}`, `${config.KEY}`, `${project.config.KEY}` (Doppler/Infisical) | **Mang sang, sửa hành vi lỗi** | Giữ cú pháp quen thuộc. Resolve trên máy chạy, lúc chạy. Đích không có replica hoặc không resolve được thì báo lỗi hay cảnh báo, không im lặng để nguyên `${...}`. |
| Missing-secret detection giữa các Environment (Doppler) | **Mang sang** | Chỉ là so sánh tập key giữa các Config cục bộ, hợp với một màn hình TUI. Chỉ so được các Config có replica. |
| Cascade thay đổi sang Environment khác theo kiểu PATCH (Doppler) | **Mang sang** (sau v1) | Là thao tác ghi bình thường lên nhiều Config, không cần server. |
| `doppler.yaml` chỉ để `setup` đọc, lưu ánh xạ thư mục ở `~/.doppler` (Doppler) | **Sửa lại** | `kith run` đọc thẳng file Setup đã commit, như `.infisical.yaml` và `.sigyn.toml`. Nhờ vậy tránh được lỗi "dời thư mục là mất cấu hình". Override cục bộ, nếu cần, để trong một file không commit. |
| `setup` nhiều entry với `path` cho monorepo (Doppler) | **Mang sang** | Ca thực tế khi một repo có nhiều app. Xem #7. |
| Named command (`[commands]` của sigyn; Doppler không có) | **Mang sang** | Đã có trong vocabulary của kith. sigyn cho thấy cách làm `kith run <name>` hợp lý. |
| `run -- cmd` và `run --command "…"` (Doppler) | **Mang sang** | UX cốt lõi để thay `.env`. |
| `--preserve-env`, `--only-secrets`, `--name-transformer` (Doppler) | **Mang sang** `--preserve-env` và `--only-secrets`; **bỏ** name-transformer (v1) | Hai flag đầu cần cho dev local. name-transformer phục vụ hệ sinh thái .NET/Terraform, chưa có nhu cầu. |
| Chặn tên biến nguy hiểm `LD_PRELOAD`, `DYLD_INSERT_LIBRARIES`… (Doppler) | **Mang sang** | Ở mô hình phân tán, giá trị đến từ máy khác. Một máy bị chiếm có thể đẩy `LD_PRELOAD` tới mọi máy nhận, nên rủi ro còn lớn hơn Doppler. |
| `--watch` restart khi secret đổi (Doppler) | **Sửa lại** (sau v1) | Kích hoạt theo sự kiện replica cập nhật, không phải theo polling server. |
| `--mount` qua FIFO 0600, không inject env, không chạy trên Windows (Doppler); `exec-file` + `--no-fifo` (SOPS) | **Sửa lại** | FIFO làm mặc định, có lựa chọn file tạm 0600 xóa khi thoát cho tool đọc nhiều lần và cho Windows. Định dạng tối thiểu: `env`, `json`. Kiểm chứng ở #6. |
| `--mount-template` / `secrets substitute` (Doppler) | **Bỏ** (v1) | Cần thêm một ngôn ngữ template. `env`/`json` đã phủ ca Flutter. |
| Fallback file mã hóa + `--offline` (Doppler); secrets-backup (Infisical) | **Sửa lại** | Trong kith, replica mã hóa at-rest **là** nguồn đọc chính, không phải bản dự phòng. Bỏ các flag `--fallback*`. |
| `secrets download` mặc định ghi file mã hóa, `--no-file` ra stdout (Doppler) | **Sửa lại** | Mục tiêu là bỏ `.env`, nên chỉ giữ `export --format env|json` ra stdout cho ca migrate hoặc debug, kèm cảnh báo. Không ghi file plaintext mặc định. |
| `secrets upload` / import ENV, JSON, YAML (Doppler) | **Mang sang** | Cần để chuyển từ `.env` sẵn có sang kith. |
| Biến dành riêng `DOPPLER_PROJECT/ENVIRONMENT/CONFIG` (Doppler) | **Mang sang** dưới tên `KITH_*` | Giúp script biết mình đang chạy với Config nào. |
| Config log kiểu `git log`, rollback = entry đảo ngược (Doppler); commit point-in-time (Infisical) | **Sửa lại** | Lịch sử là log thay đổi được nhân bản, và có chữ ký để biết máy nào ghi (age không xác thực người gửi). Rollback vẫn là một thay đổi mới. |
| Redact version cũ (Doppler) | **Sửa lại** | Không có server để "giấu", và các replica khác vẫn giữ dữ liệu cũ. Chỉ có thể là tombstone + nén lịch sử, kèm ghi rõ giới hạn. |
| Activity log workplace, SSO/SCIM, roles, change request, service token, dynamic secret, integration sync (Doppler/Infisical) | **Bỏ** | Gắn với server trung tâm hoặc tổ chức lớn, ngoài phạm vi v1. |
| Lock config chặn rename/delete (Doppler) | **Mang sang** (sau v1) | Một cờ trên Config, được nhân bản như dữ liệu. |
| Mã hóa từng value, key để rõ (SOPS, dotenvx) | **Sửa lại** | Ý tưởng diff/merge theo key là đúng. Nhưng tên key cũng có thể là thông tin nhạy cảm, và MAC toàn file làm hỏng việc gộp. kith nên lưu theo từng entry có metadata riêng. Mức lộ tên key chốt ở #4/#5. |
| Nhiều recipient bọc một data key (SOPS, age, gopass, sigyn) | **Mang sang** | Mô hình chuẩn để cấp một Config cho N máy. Recipient là khóa của từng máy, không phải của từng người. |
| Xoay data key khi bỏ recipient (SOPS `rotate`, sigyn) | **Mang sang** | Không xoay thì máy bị thu hồi vẫn giải được các thay đổi sau đó nếu nhận được ciphertext. |
| Shamir key group M-of-N (SOPS) | **Bỏ** (v1) | Phức tạp, không cần cho dev local. |
| SSH key làm recipient (age, gopass) | **Cân nhắc** ở #4 | Tận dụng khóa đã có trên máy. Nhưng gắn danh tính máy với SSH key có thể xung đột với danh tính tailnet. |
| git làm transport (dotenvx, gopass, sigyn, SOPS) | **Bỏ** | kith nhân bản trực tiếp qua mạng riêng. git cũng không giải được bài toán gộp, như sigyn đã cho thấy. |
| Một private key mỗi file, chia sẻ bằng tay (dotenvx); khóa đối xứng chung (git-crypt) | **Bỏ** | Không thu hồi được từng máy. |
| Mã hóa tất định cả file (git-crypt) | **Bỏ** | Lộ việc file có đổi hay không, và gộp được. |
| Invite token có hạn + approve, sự kiện thành viên có chữ ký, cursor để bắt kịp khi offline (env.sync.local) | **Mang sang** (tham khảo) | Khớp với bài toán thêm máy vào tailnet và cấp Config. Là đầu vào cho #4. |
| Gộp theo timestamp cả file (env.sync.local) | **Bỏ** | Mất dữ liệu khi hai máy sửa hai key khác nhau, và phụ thuộc vào đồng hồ máy. |
| LWW-Map theo key + vector clock phát hiện ghi đồng thời + chiến lược giải (sigyn) | **Sửa lại** | Là hình dạng metadata nên dùng, nhưng cần cài đặt chạy thật: tombstone khi xóa, hiện xung đột trong TUI thay vì âm thầm lấy bản mới hơn. Chốt ở #5. |
| `run serve` cấp secret qua Unix socket (sigyn) | **Bỏ** (v1) | Ứng dụng phải sửa code để đọc socket, trái với mục tiêu "chỉ cần `kith run -- cmd`". |
| `scan` / `redact` (Teller) | **Bỏ** (v1), ghi nhận | Không thuộc lõi nhân bản. `redact` có thể hữu ích cho log của Named command về sau. |

## 5. Câu hỏi mở chuyển cho ticket sau

- #4: Cấp branch config có tự động cấp root không? Personal config trong kith là "branch chỉ nhân bản tới máy của tôi" hay là một khái niệm riêng? Recipient là khóa máy (age X25519), SSH key, hay khóa dẫn từ danh tính tailnet? Tên key có được mã hóa không?
- #5: Đơn vị gộp là từng key, gồm value, clock, writer và tombstone. Khi hai máy sửa cùng một key: tự chọn một bên theo LWW hay bắt người dùng giải trong TUI? Rollback được lan truyền như thế nào?
- #6: Kiểm tra `flutter --dart-define-from-file` (và Gradle, Xcode nếu chúng nhận file) với FIFO: tool đọc mấy lần, có `stat` kích thước không. Quyết định mặc định FIFO hay file tạm.
- #7: Định dạng Setup: một file commit có `project`, `config`, `path` cho nhiều app và `commands`. Override cục bộ không commit đặt ở đâu?
