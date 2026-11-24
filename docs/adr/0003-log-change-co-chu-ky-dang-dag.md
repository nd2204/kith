# Nhân bản bằng log Change có chữ ký dạng DAG, gộp theo từng key

kith không có server, và máy ghi được cả khi offline. Vì vậy mỗi Config, và cả phần thành viên cùng Grant của mỗi Project, là một log **Change** chỉ-thêm. Mỗi Change do một Machine ký bằng Ed25519 và trỏ tới hash của các Change đầu mà máy đó đã thấy, giống commit của git. Gộp chỉ là hợp hai tập Change lại; trạng thái hiện tại được tính lại từ log. Quan hệ nhân quả do DAG quyết định. HLC trong Change chỉ dùng để xếp các Change đồng thời. Hai Change đồng thời sửa cùng một key tạo ra một **Conflict**: kith chọn tạm một giá trị theo quy tắc tất định (HLC lớn hơn thắng, hòa thì so public key của Machine), để `run` không bao giờ bị chặn, và Conflict vẫn hiển thị cho tới khi có người chọn giá trị đúng.

## Considered Options

- **State CRDT LWW-Map kèm vector clock** (thiết kế của sigyn): bị loại, vì không có lịch sử và audit, và vì LWW âm thầm làm mất một giá trị secret đúng.
- **Snapshot cả Config so timestamp** (env.sync.local, SOPS): bị loại, vì hai máy sửa hai key khác nhau cũng ghi đè lên nhau.
- **Chặn ghi khi không đủ peer** (quorum): bị loại, vì nhiều Config chỉ có một hoặc hai máy giữ, và vì trái nguyên tắc local-first.
- **Ký cả trong lẫn ngoài** (ký plaintext, mã hóa, rồi ký envelope): bị loại, vì data key là khóa đối xứng nên Machine có Grant vẫn bóc được chữ ký trong và mã hóa lại dưới tên mình; cách này không chặn thêm gì so với AAD mà tốn gấp đôi việc ký.

## Consequences

- Change được mã hóa trước rồi mới ký, và ciphertext được ràng buộc vào header. Header gồm `format`, id Project và Config, epoch data key, Machine tác giả và các hash cha; nó được truyền và lưu dưới dạng byte nguyên vẹn, không bao giờ dựng lại từ các trường đã parse. Payload (tên key, giá trị, HLC) mã hóa bằng XChaCha20-Poly1305 với nonce ngẫu nhiên, và đúng các byte của header là associated data. Hash là `SHA-256(header ‖ nonce ‖ ciphertext)`; chữ ký Ed25519 của tác giả ký lên hash kèm tiền tố phân tách miền `kith/change/v1` (log Project dùng cùng khung, không có ciphertext, tiền tố `kith/project-change/v1`). Tên key không lộ qua metadata gộp.
- Nếu không ràng buộc như vậy, Machine nào thấy ciphertext cũng gỡ được chữ ký gốc, ký lại dưới tên mình hoặc ghép vào chỗ cha, Config hay epoch khác, và "cứu" được Change của máy đã bị thu hồi mà không để lại dấu vết ([Defective Sign & Encrypt](http://world.std.com/~dtd/sign_encrypt/sign_encrypt7.html), mục 1.2). Với AAD, việc đó làm giải mã thất bại; muốn dùng lại một giá trị thì phải ghi Change mới đứng tên mình.
- Hash, chữ ký và quyền được kiểm khi nhận Change, không cần key. AAD chỉ kiểm được khi giải mã: máy có key giải mã thử ngay lúc nhập, máy chưa có key (vừa được Grant, hoặc đang `backup import`) đánh dấu "chưa kiểm nội dung" và kiểm khi key tới. Change có chữ ký hợp lệ mà không giải mã được thì bị loại như Change của máy bị thu hồi: vẫn là nút trong DAG và vẫn được nhân bản, nhưng không đóng góp giá trị, không tạo Conflict. TUI và `kith doctor` cảnh báo đích danh Machine đã ký; kith không tự thu hồi máy đó.
- Quyền được xét theo quá khứ nhân quả: một Change chỉ hợp lệ nếu tác giả có Grant (hoặc là Admin, với log Project) xét theo các Change cha của nó. Hai Admin thu hồi lẫn nhau cùng lúc thì Admin có thâm niên cao hơn thắng. Grant và lệnh thu hồi đến đồng thời thì thu hồi thắng. Change của máy bị thu hồi mà không nằm trong quá khứ nhân quả của lệnh thu hồi thì bị loại, nhưng vẫn hiển thị để người dùng áp dụng lại bằng tay.
- Xoay data key tạo một epoch mới, và key của epoch mới bọc luôn key của epoch trước. Nhờ vậy mọi Machine đang có Grant đọc được toàn bộ lịch sử, còn máy đã bị thu hồi không đọc được epoch mới.
- Log lớn dần theo thời gian, nên sau này cần compaction.
