# Ký release bằng minisign với hai khóa ghim trong binary

kith giữ secret prod, nên binary phát hành phải được kiểm là do dự án ký. Mỗi release ký `checksums.txt` bằng minisign (Ed25519). Binary kith ghim sẵn **hai** public key: một **key đang dùng**, có private key nằm trong secret của GitHub Actions và ký mọi release, và một **key dự phòng** giữ offline. `kith self-update` luôn kiểm chữ ký bằng hai key này và từ chối nếu chữ ký không hợp lệ. Còn `install.sh` và `install.ps1` bắt buộc kiểm SHA-256, và chỉ kiểm chữ ký khi máy có sẵn `minisign`: script và checksum tải qua TLS từ cùng một nơi, nên chữ ký chỉ thật sự bảo vệ được khi binary đã cài sẵn là nguồn tin cậy.

## Considered Options

- **Chỉ có checksum, không ký**: bị loại. Ai chiếm được quyền ghi release, hoặc đánh tráo asset, thì `self-update` sẽ cài binary của họ lên mọi máy đang giữ secret.
- **cosign keyless (Sigstore, OIDC của GitHub)**: không phải giữ khóa, nhưng kiểm trong Go cần sigstore-go, kéo theo một cây phụ thuộc lớn, và lệnh cài cần người dùng có sẵn cosign. Bị loại vì cái giá đó quá lớn cho một CLI static.
- **Ghim một key duy nhất**: bị loại. Nếu mất hoặc lộ key, mọi bản đã cài sẽ không còn tự nâng cấp an toàn được, và người dùng phải cài lại bằng tay.

## Consequences

- Nếu key đang dùng bị lộ hoặc mất, ta ký bằng key dự phòng một release ghim cặp khóa mới. Bản cũ vẫn `self-update` được.
- Private key dự phòng phải được cất offline và không bao giờ đưa vào CI. Mất cả hai key thì người dùng phải cài lại bằng `install.sh`.
- Khóa đã ghim nằm trong mọi binary đã phát hành, nên đổi sang cách ký khác cần một release chuyển tiếp ký bằng khóa cũ.
