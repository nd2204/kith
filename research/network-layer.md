# Research: lớp mạng riêng cho kith

Ticket: [#2](https://github.com/nd2204/kith/issues/2). Ngày tra cứu: 2026-11-19. Ghi chú này chỉ gom dữ kiện và đưa ra khuyến nghị. Quyết định chốt thuộc về phiên grilling sau.

## Câu hỏi

kith nhân bản secret giữa các máy tin cậy qua một mạng riêng có sẵn và không tự xây mesh. Nên tích hợp lớp mạng theo cách nào?

- **(a)** Giả định máy đã ở trong tailnet. kith chỉ dùng IP/MagicDNS, có thể hỏi `tailscaled` qua LocalAPI.
- **(b)** Nhúng `tsnet` để kith tự join tailnet (Tailscale SaaS hoặc Headscale qua `ControlURL`).
- **(c)** Lớp vận chuyển cắm được: Tailscale, Headscale, NetBird, ZeroTier, LAN thuần.

## Tóm tắt

1. **Danh tính** chỉ thật sự giàu ở họ Tailscale/Headscale. `WhoIs` trả `Node`, `UserProfile` và `CapMap` (app capability từ grants), và Headscale cũng biên dịch được app grants. NetBird embed chỉ trả public key và FQDN của peer. ZeroTier không có API tương đương ở phía client.
2. **Tag xoá danh tính người dùng** của máy. Máy đã gắn tag thì `WhoIs` không còn cho biết người nào. Vì vậy mô hình quyền không thể dựa cùng lúc vào "máy có tag prod" và "người là ai" trên cùng một node.
3. **tsnet làm binary tăng khoảng 15–16 MB.** Đo trên tailscale.com v1.104.7, probe tối thiểu nặng khoảng 20,5–22,5 MB, so với khoảng 5,5–6 MB của chương trình `net/http` trống. Bỏ bớt feature tag chỉ giảm được khoảng 0,6 MB. Cross-compile `CGO_ENABLED=0` chạy được cho darwin/linux/windows.
4. **Khi control plane chết**, kết nối đang chạy vẫn sống. Node khởi động lại thì mất peer, trừ khi bật netmap caching (opt-in, client ≥ 1.104, cần nodeAttr `cache-network-maps`, không áp dụng cho ephemeral node). kith luôn đọc được từ bản sao cục bộ, nên điều này chỉ ảnh hưởng tới việc nhân bản, không ảnh hưởng tới việc đọc.
5. **Gói Personal của Tailscale chỉ dành cho dùng phi thương mại** (≤ 6 user). Dùng cho công việc thì cần Standard ($8/user/tháng) hoặc tự host Headscale (BSD-3, một tailnet).
6. **Khuyến nghị:** chọn **(a) làm mặc định của v1**. Đặt một seam `Transport` hẹp trong code, để sau này cắm (b) tsnet hoặc chế độ "địa chỉ thuần" (LAN/NetBird/ZeroTier) mà không phải viết lại. Danh tính và mã hoá secret nên nằm ở chính kith (khoá máy riêng, mã hoá đầu-cuối tới máy được cấp). Danh tính mạng chỉ là lớp phụ trợ. Chi tiết và lý do ở [Khuyến nghị](#khuyến-nghị-cho-grilling).

## 1. Danh tính máy/người mà mạng cung cấp

### Tailscale: WhoIs, CapMap, grants

- `local.Client.WhoIs(ctx, remoteAddr)` trả `*apitype.WhoIsResponse`. Lỗi `ErrPeerNotFound` nghĩa là không tìm thấy peer. [pkg.go.dev client/local][lc]
- `WhoIsResponse` có ba trường: `Node *tailcfg.Node`, `UserProfile *tailcfg.UserProfile`, `CapMap tailcfg.PeerCapMap`. [source `client/tailscale/apitype/apitype.go` v1.104.7][src]
- Handler `whois` của LocalAPI chỉ đòi `PermitRead`. User thường trên máy nên đọc được, không cần quyền ghi hay operator. [source `ipn/localapi/localapi.go` `serveWhoIsWithBackend`][src]
- Grants gộp quyền mạng (`ip`) và quyền tầng ứng dụng (`app`). Tailscale nói rõ: "Application layer capabilities are defined by the applications themselves, not by Tailscale". Tham số là JSON mờ mà policy engine không kiểm tra. Ứng dụng đọc capability từ request đi qua tailnet. [KB Grants][grants]
- Hệ quả: kith có thể đặt capability riêng trong policy của tailnet, ví dụ `kith.dev/cap/config` mang `{"configs": ["proj/prod"]}`, rồi đọc qua `WhoIs(...).CapMap`. Lúc đó policy của tailnet trở thành nơi khai báo quyền.

### Tags

- "Applying a tag to a device removes any user-based authentication" và "authenticating a device with a user account removes all tags from the device". Một máy chỉ có danh tính theo tag hoặc theo user, không bao giờ có cả hai. [KB Tags][tags]
- Máy gắn tag mặc định tắt key expiry. Máy theo user có key hết hạn sau 180 ngày, và khi đó "connections to/from the given endpoint will stop working". [KB Key expiry][expiry]
- Hệ quả cho mô hình quyền: laptop dev thường là node theo user (biết người), server/CI thường là node gắn tag (biết vai trò, không biết người). Mô hình quyền phải chọn hoặc chấp nhận cả hai loại chủ thể.

### Headscale

- Hỗ trợ ACLs, Grants (khuyên dùng Grants vì "ACLs are considered legacy"), tags, ephemeral node, auth key, MagicDNS, Tailscale SSH, node attributes. Không hỗ trợ device posture và IP sets. Autogroups chỉ có một phần. [Headscale features][hs-feat], [Headscale policy][hs-policy]
- App grants có thật trong code: `hscontrol/policy/v2/compiled.go` sinh `tailcfg.CapGrant` từ trường `App` (commit a491228, 2026-11-16). Release v0.29.5 có sửa lỗi "app grants dropping a group that names an unknown user". [source Headscale][hs-repo], [releases][hs-rel]
- Không thấy tài liệu Headscale nào nhắc Tailnet Lock (đã grep `docs/`). Nên coi là không có.

### NetBird và ZeroTier

- NetBird `client/embed` có `IdentityForIP(ip)`, chỉ trả WireGuard public key và FQDN của peer đang hoạt động. Nó không trả user và không có capability tầng ứng dụng. [pkg.go.dev netbird embed][nb-embed]
- ZeroTier libzt không có binding Go. Danh tính chỉ ở mức thành viên mạng. Không tìm thấy API client nào trả user hay capability. [libzt][libzt]

## 2. Nhúng tsnet: khả thi và kích thước

### API

- `tsnet.Server` có các trường `Dir`, `Store`, `Hostname`, `Ephemeral`, `AuthKey`, `ControlURL`, `AdvertiseTags`, `Port`, OAuth/workload identity. Các method gồm `Listen`, `ListenTLS`, `Dial`, `LocalClient`, `Up`, `Start`, `Close`. License BSD-3-Clause. [pkg.go.dev tsnet][tsnet-pkg]
- `ControlURL` lấy theo thứ tự: trường đã đặt, rồi biến môi trường `TS_CONTROL_URL`, rồi mặc định của Tailscale. Nghĩa là trỏ sang Headscale chỉ là một chuỗi cấu hình. [pkg.go.dev tsnet][tsnet-pkg]
- `AuthKey` bị bỏ qua khi node đã có state. Không có key thì tsnet in ra URL đăng nhập. [pkg.go.dev tsnet][tsnet-pkg], [KB tsnet][tsnet-kb]
- `Dir` mặc định nằm dưới `os.UserConfigDir` và đặt theo tên binary. Muốn chạy nhiều service trong một binary thì mỗi service phải có `Dir` riêng. [source `tsnet/tsnet.go`][src] Suy ra: một máy chỉ nên có **một tiến trình kith giữ node tsnet** (agent). Các lệnh `kith run`/`secrets get` không được tự dựng node mỗi lần gọi.
- tsnet dùng userspace TCP/IP stack, nên không cần TUN hay quyền admin. Mỗi tiến trình là **một device riêng** trong tailnet. [KB tsnet][tsnet-kb]

### Đo kích thước binary

Môi trường đo: Go 1.27.0, `tailscale.com v1.104.7`, `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`. Đơn vị MB (10^6 byte). Chưa tính Bubble Tea. Probe tsnet gồm `Listen` + `LocalClient().WhoIs`.

| Chương trình | darwin/arm64 | linux/amd64 | windows/amd64 |
|---|---|---|---|
| `net/http` trống (baseline) | 5,53 | 5,82 | 6,01 |
| chỉ `client/local` + `WhoIs` (phương án a) | 8,15 | 8,64 | 8,90 |
| tsnet probe (phương án b) | 20,55 | 22,25 | 22,50 |
| tsnet probe, bỏ ~23 feature tag `ts_omit_*` | 19,94 | 21,55 | 21,83 |

- tsnet linux/amd64 nén `gzip -9` còn khoảng 8,7 MB. Đây là cỡ file tải về qua `install.sh` nếu release dạng `.tar.gz`.
- Không omit được `serve`/`webclient`/`acme` khi dùng tsnet: build lỗi vì `tsnet.go` gọi `ServeConfig`. Các tag còn lại (aws, kube, ssh, taildrop, drive, relayserver, clientupdate, exit node…) bỏ được nhưng gần như không giảm kích thước. Danh sách tag nằm ở `feature/featuretags/featuretags.go`. [source][src]
- Tailscale có hướng dẫn thu nhỏ binary, dùng UPX cho `tailscaled`, nhưng cũng cảnh báo packer có thể làm giảm an toàn. [KB Smaller binaries][small]

## 3. Khi offline hoặc control plane chết

- Kết nối đang có vẫn chạy: "all your existing connections keep working". Phần không chạy là "adding/removing/changing nodes and packet filters". Client **khởi động lại** trong lúc control plane chết sẽ "forgets its network map and falls off the network". [Blog Tailscale, 2026-01-21][hypergrowth]
- Netmap caching cho phép node khởi động hoặc nối lại peer khi không tới được control plane. Các điều kiện: client ≥ 1.104; **opt-in** qua nodeAttr `cache-network-maps` trong policy; cần đĩa lưu bền; đã từng nối control plane ít nhất một lần. Giới hạn: không có cho mobile và ephemeral node, không nối được tới peer mới join sau thời điểm cache, key đã hết hạn thì vẫn phải lên control plane. [Docs netmap caching][nmcache]
- Code tsnet v1.104.7 dùng chung `ipnlocal` có `diskcache.go`. Cache chỉ được ghi khi node có cap `CacheNetworkMaps` và `TS_USE_CACHED_NETMAP` không tắt. [source `ipn/ipnlocal/local.go`][src] Vì vậy **suy ra** tsnet cũng hưởng netmap caching khi policy cấp attr. Tài liệu không nói rõ điểm này, cần thử thực tế.
- Headscale cho dùng "other key-only node attributes" ngoài danh sách đã liệt kê. Về lý thuyết có thể đặt `cache-network-maps` trong `nodeAttrs`, nhưng chưa kiểm chứng. [Headscale policy, footnote 2][hs-policy]
- Với kith, nguyên tắc "đọc từ bản sao cục bộ luôn được" không phụ thuộc lớp mạng. Control plane chết chỉ làm chậm việc **nhân bản** giữa các máy vừa khởi động lại, không làm hỏng việc đọc hay `run`.
- LAN không có Internet: Tailscale/Headscale vẫn cần control plane để có netmap, trừ khi có cache. Chế độ "LAN thuần" của (c) là cách duy nhất chạy khi không có control plane nào.

## 4. License và chi phí

| Thành phần | License | Ghi chú |
|---|---|---|
| tailscale (tsnet, client/local, tailscaled) | BSD-3-Clause | GUI wrapper trên nền tảng không mã nguồn mở (macOS, Windows, iOS) không mở mã. [README tailscale][src] |
| Tailscale SaaS, gói Personal | miễn phí | ≤ 6 user, user device không giới hạn, 50 tagged resource, 1.000 phút ephemeral/tháng, 3 ACL group. "only suitable for non-commercial use". Tailnet dùng custom domain bị coi là business use. Giới hạn tag/ephemeral hiện chưa bị chặn. [Pricing][pricing] |
| Tailscale SaaS, gói Standard | $8/user/tháng | user không giới hạn, 10 ACL group. [Pricing][pricing] |
| Headscale | BSD-3-Clause | "a _single_ Tailscale network", phù hợp "personal use, or a small open-source organisation". Không liên kết với Tailscale Inc. Client tối thiểu v1.80.0 (v0.29.6/7). Cần tự host một máy có thể truy cập được. [repo][hs-repo], [releases][hs-rel] |
| NetBird | client BSD-3; `management/`, `signal/`, `relay/`, `combined/` AGPLv3 | Cloud free 5 user, 100 máy; Team €6/user/tháng. [LICENSE][nb-lic], [pricing][nb-price] |
| ZeroTier libzt | BSL 1.1 | ứng dụng đóng mã thương mại cần license thương mại; không có binding Go. [libzt][libzt] |

Lưu ý: Headscale là control plane tự host. Nó chỉ điều phối khoá và netmap, không lưu secret, nên không mâu thuẫn với nguyên tắc "không có server lưu secret tập trung". Tuy vậy nó vẫn là một dịch vụ phải vận hành.

## 5. Hỗ trợ nền tảng

- tsnet probe cross-compile `CGO_ENABLED=0` thành công cho darwin/arm64, linux/amd64 và windows/amd64 (đo ở mục 2). tsnet chạy userspace nên không cần driver WinTun hay quyền admin.
- `client/local` tự tìm LocalAPI theo từng nền tảng: Windows dùng named pipe `\\.\pipe\ProtectedPrefix\Administrators\Tailscale\tailscaled`, macOS dùng `/var/run/tailscaled.socket` hoặc cơ chế token `sameuserproof` cho bản App Store/standalone. [source `paths/paths.go`, `safesocket/safesocket_darwin.go`][src] Theo tài liệu, chỉ macOS dùng auth token. [pkg.go.dev client/local][lc]
- Headscale ghi Windows và macOS là client được hỗ trợ. [Headscale clients][hs-repo]

## So sánh ba phương án

| Tiêu chí | (a) tailnet có sẵn | (b) nhúng tsnet | (c) cắm được |
|---|---|---|---|
| Cài đặt cho người dùng | phải cài và đăng nhập Tailscale trước | chỉ cần kith; lần đầu mở URL login hoặc dùng auth key | tuỳ transport |
| Kích thước binary (linux/amd64) | ~8,6 MB nếu dùng `client/local`, ~5,8 MB nếu chỉ dùng IP | ~22 MB | ít nhất bằng transport lớn nhất được build vào |
| Danh tính | `WhoIs` của máy host (user hoặc tag) + CapMap | `WhoIs` của node kith riêng (user hoặc tag) + CapMap | Tailscale/Headscale có; NetBird chỉ có pubkey/FQDN; ZeroTier/LAN không có |
| Device trong tailnet | dùng chung device của máy | thêm một device mỗi máy; key hết hạn 180 ngày nếu theo user | tuỳ |
| Control plane chết | giống Tailscale (tailscaled sống lâu, ít khi restart) | giống Tailscale, nhưng node khởi động lại theo agent kith | LAN thuần không cần control plane |
| Headscale | có, nếu máy đã join Headscale | có, qua `ControlURL` | có |
| Windows | phụ thuộc client Tailscale | userspace, không cần admin | tuỳ |
| Khối lượng code/test | nhỏ nhất | vừa: agent, state dir, login flow | lớn nhất: ma trận transport × nền tảng |
| Rủi ro | phụ thuộc cấu hình tailnet của người dùng (ACL chặn port, MagicDNS tắt) | binary to; thêm một device phải duyệt; quản lý state tsnet | phình phạm vi v1, danh tính không đồng nhất |

## Khuyến nghị cho grilling

Đây là khuyến nghị. Quyết định cuối thuộc về phiên grilling.

1. **v1 chọn (a).** kith agent lắng nghe trên IP tailnet của máy. Peer được tìm qua MagicDNS/IP. Danh tính peer lấy từ `client/local.WhoIs`, có thể đọc CapMap. Đây là phương án nhỏ nhất, đúng nguyên tắc "không tự xây mesh", và chạy được với cả Tailscale lẫn Headscale.
2. **Giữ một seam `Transport` hẹp** (Listen, Dial, PeerIdentity) ngay từ đầu, để (b) tsnet là một implementation bổ sung chứ không phải một lần viết lại. Chỉ bật tsnet khi có nhu cầu thật, ví dụ máy không thể cài Tailscale hệ thống, hoặc cần node kith tách khỏi device người dùng.
3. **Không làm (c) đầy đủ trong v1.** Nếu cần, chỉ thêm một chế độ "địa chỉ thuần": danh sách IP/hostname cấu hình tay, dùng cho LAN, NetBird hay ZeroTier. Chế độ này dựa hoàn toàn vào danh tính của chính kith.
4. **Danh tính và mã hoá secret nên thuộc về kith**, không phó mặc cho lớp mạng. Ba lý do: tag xoá user identity; Headscale không có Tailnet Lock; ai sửa được policy tailnet thì cấp được CapMap. Mỗi máy có khoá riêng, và Config được mã hoá tới đúng các máy được cấp. `WhoIs`/CapMap chỉ dùng làm lớp kiểm tra thêm và để tiện ghi danh máy mới. Phần này để ticket mô hình quyền chốt.
5. **Dù chọn gì, kith cũng cần một agent sống lâu trên mỗi máy** để nhận bản sao, và agent nên bật netmap caching trong policy nếu muốn nhân bản sống sót qua lúc control plane chết.

Đánh đổi chính khi chọn (a): người dùng phải tự cài Tailscale, nên bước onboarding có thêm một bước. Bù lại, binary nhỏ hơn khoảng 13 MB, không phát sinh thêm device, và không phải vận hành state tsnet.

## Câu hỏi mở cho grilling

- Chủ thể của quyền là **máy** hay **người** hay cả hai? Server/CI gắn tag thì mất danh tính người.
- Quyền khai báo ở **policy tailnet** (CapMap) hay ở **dữ liệu của kith** (ký bởi admin kith)? Hay kết hợp cả hai?
- Có chấp nhận yêu cầu bắt buộc Tailscale/Headscale cho v1 không? Có cần chế độ LAN thuần ngay không?
- Tailscale SaaS hay Headscale cho nhóm đầu tiên? Ràng buộc "non-commercial" của gói Personal có áp dụng không?
- CI/deploy ngoài tailnet (ticket sau) có làm thay đổi lựa chọn này không? Ví dụ tsnet + auth key ephemeral rất hợp với runner CI, nhưng ephemeral node lại không có netmap cache.

## Nguồn

- [lc]: https://pkg.go.dev/tailscale.com/client/local
- [tsnet-pkg]: https://pkg.go.dev/tailscale.com/tsnet
- [tsnet-kb]: https://tailscale.com/kb/1244/tsnet
- [grants]: https://tailscale.com/kb/1324/grants
- [tags]: https://tailscale.com/kb/1068/tags
- [expiry]: https://tailscale.com/kb/1028/key-expiry
- [pricing]: https://tailscale.com/pricing
- [hypergrowth]: https://tailscale.com/blog/hypergrowth-isnt-always-easy
- [nmcache]: https://tailscale.com/docs/reference/client-side-netmap-caching
- [small]: https://tailscale.com/kb/1207/small-tailscale
- [src]: mã nguồn `tailscale.com@v1.104.7` (module Go; https://github.com/tailscale/tailscale)
- [hs-repo]: https://github.com/juanfont/headscale (shallow clone commit a491228, 2026-11-16)
- [hs-feat]: https://headscale.net/stable/about/features/
- [hs-policy]: https://headscale.net/stable/ref/policy/
- [hs-rel]: https://github.com/juanfont/headscale/releases
- [nb-embed]: https://pkg.go.dev/github.com/netbirdio/netbird/client/embed
- [nb-lic]: https://github.com/netbirdio/netbird/blob/main/LICENSE
- [nb-price]: https://netbird.io/pricing
- [libzt]: https://github.com/zerotier/libzt

[lc]: https://pkg.go.dev/tailscale.com/client/local
[tsnet-pkg]: https://pkg.go.dev/tailscale.com/tsnet
[tsnet-kb]: https://tailscale.com/kb/1244/tsnet
[grants]: https://tailscale.com/kb/1324/grants
[tags]: https://tailscale.com/kb/1068/tags
[expiry]: https://tailscale.com/kb/1028/key-expiry
[pricing]: https://tailscale.com/pricing
[hypergrowth]: https://tailscale.com/blog/hypergrowth-isnt-always-easy
[nmcache]: https://tailscale.com/docs/reference/client-side-netmap-caching
[small]: https://tailscale.com/kb/1207/small-tailscale
[src]: https://github.com/tailscale/tailscale/tree/v1.104.7
[hs-repo]: https://github.com/juanfont/headscale
[hs-feat]: https://headscale.net/stable/about/features/
[hs-policy]: https://headscale.net/stable/ref/policy/
[hs-rel]: https://github.com/juanfont/headscale/releases
[nb-embed]: https://pkg.go.dev/github.com/netbirdio/netbird/client/embed
[nb-lic]: https://github.com/netbirdio/netbird/blob/main/LICENSE
[nb-price]: https://netbird.io/pricing
[libzt]: https://github.com/zerotier/libzt
