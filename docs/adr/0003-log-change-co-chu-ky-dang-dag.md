# Nhân bản bằng log Change có chữ ký dạng DAG, gộp theo từng key

kith không có server, và máy ghi được cả khi offline. Vì vậy mỗi Config, và cả phần thành viên cùng Grant của mỗi Project, là một log **Change** chỉ-thêm. Mỗi Change do một Machine ký bằng Ed25519 và trỏ tới hash của các Change đầu mà máy đó đã thấy, giống commit của git. Gộp chỉ là hợp hai tập Change lại; trạng thái hiện tại được tính lại từ log. Quan hệ nhân quả do DAG quyết định. HLC trong Change chỉ dùng để xếp các Change đồng thời. Hai Change đồng thời sửa cùng một key tạo ra một **Conflict**: kith chọn tạm một giá trị theo quy tắc tất định (HLC lớn hơn thắng, hòa thì so public key của Machine), để `run` không bao giờ bị chặn, và Conflict vẫn hiển thị cho tới khi có người chọn giá trị đúng.

## Considered Options

- **State CRDT LWW-Map kèm vector clock** (thiết kế của sigyn): bị loại, vì không có lịch sử và audit, và vì LWW âm thầm làm mất một giá trị secret đúng.
- **Snapshot cả Config so timestamp** (env.sync.local, SOPS): bị loại, vì hai máy sửa hai key khác nhau cũng ghi đè lên nhau.
- **Chặn ghi khi không đủ peer** (quorum): bị loại, vì nhiều Config chỉ có một hoặc hai máy giữ, và vì trái nguyên tắc local-first.

## Consequences

- Change được mã hóa trước rồi mới ký. Envelope để ngoài chỉ gồm id Project và Config, hash, các hash cha, Machine tác giả, epoch data key và chữ ký. Tên key, giá trị và HLC nằm trong phần mã hóa, nên metadata gộp không làm lộ tên key.
- Quyền được xét theo quá khứ nhân quả: một Change chỉ hợp lệ nếu tác giả có Grant (hoặc là Admin, với log Project) xét theo các Change cha của nó. Hai Admin thu hồi lẫn nhau cùng lúc thì Admin có thâm niên cao hơn thắng. Grant và lệnh thu hồi đến đồng thời thì thu hồi thắng. Change của máy bị thu hồi mà không nằm trong quá khứ nhân quả của lệnh thu hồi thì bị loại, nhưng vẫn hiển thị để người dùng áp dụng lại bằng tay.
- Xoay data key tạo một epoch mới, và key của epoch mới bọc luôn key của epoch trước. Nhờ vậy mọi Machine đang có Grant đọc được toàn bộ lịch sử, còn máy đã bị thu hồi không đọc được epoch mới.
- Log lớn dần theo thời gian. v1 không compaction mà giữ lịch sử vĩnh viễn, xem [ADR 0009](./0009-giu-lich-su-vinh-vien-archive-thay-cho-xoa.md).
