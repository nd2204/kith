# Giữ lịch sử vĩnh viễn: không compaction, không purge, archive thay cho xóa

Log Change của mỗi Config và log Project được giữ đầy đủ, mãi mãi, trên mọi Machine giữ chúng. kith v1 không có compaction, không có cách purge một giá trị khỏi lịch sử, và không xóa thật Config hay Project. Thay vào đó, Admin **Archive** chúng bằng một Change trong log Project: Config đã archive không chạy hay ghi được nữa, nhưng log, Grant và mọi cơ chế khác giữ nguyên, nên bật lại được. Lý do là trong một DAG nhân bản không có server, xóa không bảo đảm được: peer đang offline, file `backup export` và Recovery machine vẫn giữ bản sao, và Change đồng thời đến sau sẽ không có chỗ để vào. Một lệnh xóa không thực hiện được lời hứa của nó còn tệ hơn là không có lệnh xóa.

## Considered Options

- **Compaction log Config bằng snapshot do Machine có Grant ký**: bị loại cho v1. Ước lượng một Change cỡ 0,5–1 KB, một Config đổi 20 lần mỗi tuần thì 5 năm mới cỡ 5 MB, và trạng thái đã có cache. Phần khó là điểm cắt phải được ký sao cho máy mới vẫn kiểm được quyền nhân quả, mà ở quy mô này nó chưa giải quyết vấn đề gì thật.
- **Compaction cả log Project**: bị loại, vì quyền nhân quả của mọi Change dựa trên log Project đầy đủ.
- **Purge giá trị lỡ ghi** (tombstone payload mà mọi máy tôn trọng): bị loại, vì không gỡ được bản sao đã nhân bản hay đã sao lưu, nên chỉ tạo cảm giác an toàn giả.
- **Xóa thật Config hay Project**: bị loại, cùng lý do.

## Consequences

- Mọi giá trị từng được `set` nằm vĩnh viễn trong lịch sử của Config, và đọc được bởi mọi Machine có Grant, hiện tại và tương lai. Tài liệu ghi rõ điều này. Lỡ ghi nhầm một secret thì cách xử lý duy nhất là xoay nó ở nguồn. `kith secrets flag <KEY>` (có `--all`) gắn cờ "cần xoay secret" bằng tay, còn `kith secrets unflag <KEY>` gỡ cờ. Cả hai là Change trong log Config. Cờ, dù gắn bằng tay hay tự sinh khi thu hồi hoặc gỡ Machine, hết khi có một `set` key đó nằm sau cờ theo quan hệ nhân quả. Rollback và `reapply` không tính là `set` để gỡ cờ, và khi khôi phục một giá trị từng bị gắn cờ thì tự gắn lại cờ cho key đó.
- `kith doctor` và TUI hiện số Change và dung lượng của từng log. `doctor` cảnh báo khi một log vượt 50.000 Change hoặc 50 MB; ngưỡng chỉnh được bằng cấu hình cục bộ. Đó là tín hiệu để xem lại quyết định này. Compaction sau này, nếu có, là một loại Change mới đi qua Format floor ([ADR 0008](./0008-tuong-thich-phien-ban-bang-khung-header-co-dinh-va-format-floor.md)), nên v1 không phải chừa trước trường nào.
- **Archive**:
  - Chỉ Admin archive hay unarchive được.
  - Archive một Config gốc thì archive luôn các Branch config của nó. Archive Project thì archive mọi Config.
  - Unarchive chỉ bật lại những gì bị archive bởi đúng Change đó. Không unarchive được một Branch config khi Config gốc của nó còn archive.
  - Với Config đã archive, `run`, `use`, `secrets set/unset` và `download` từ chối và chỉ cách unarchive. `secrets get`, xem lịch sử và `backup export` vẫn chạy.
  - Máy offline chưa thấy lệnh archive thì vẫn chạy bình thường. Change ghi vào Config đã archive vẫn hợp lệ.
  - Nhân bản, Grant, xoay data key, cờ cần xoay và Conflict hoạt động như với Config thường. Ngoại lệ duy nhất: bỏ cảnh báo "chỉ còn một bản sao".
  - `doctor` liệt kê các Setup và Active config đang trỏ tới Config đã archive.
  - Lệnh: `kith configs archive|unarchive`, `kith projects archive|unarchive`, `kith configs list --archived`. TUI ẩn Config và Project đã archive theo mặc định; một phím bật hiện chúng mờ, kèm nhãn `archived`.
- Muốn bỏ dữ liệu khỏi một máy thì gỡ Grant hoặc gỡ Machine, như cơ chế đã có.
- Change bị loại (từ máy đã bị thu hồi, hoặc dưới Format floor) và Change không đọc được đều được giữ mãi và vẫn nhân bản, để tập Change hội tụ giữa các máy. "Bỏ qua" một Change bị loại trong TUI chỉ là đánh dấu cục bộ. Không có GC.
