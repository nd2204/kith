# PROTOTYPE: bố cục TUI kith (bỏ đi sau khi chốt)

Bản nháp này trả lời câu hỏi của [Bố cục TUI Bubble Tea của kith](https://github.com/nd2204/kith/issues/9). Nó **không phải code thật**: không mã hóa, không đồng bộ, toàn bộ dữ liệu là dữ liệu giả trong bộ nhớ (`data.go`). Nhánh `prototype/tui-layout` sẽ không được merge.

## Chạy

```sh
cd prototypes/tui-layout
go run .                 # tương tác
go run . --variant=C     # mở thẳng một biến thể
go run . --dump          # in các màn hình mẫu ra stdout, không cần TTY
```

Nếu chỉ muốn xem, đọc [`screens.txt`](screens.txt). Đó là bản `--dump` đã bỏ màu.

Thanh tím ở đáy là **công cụ chuyển biến thể** của prototype, không thuộc thiết kế. Bấm `<` / `>` để đổi biến thể. Ba biến thể dùng chung một trạng thái, nên sửa secret hay cấp quyền ở biến thể này thì sang biến thể khác vẫn thấy.

Dữ liệu giả có sẵn đủ các tình huống đã chốt ở các ticket trước:

- Conflict `STRIPE_KEY` ở `vet45-erp/dev` (bob-mbp vs alice-mini)
- `stg`: đã loại 2 Change từ máy đã thu hồi
- `prd`: thu hồi đang chờ xoay khóa, kèm danh sách key cần xoay
- `dev_alice` là Branch config (key kế thừa và key ghi đè)
- `vet45-shared/prd` là Config máy này không có Grant
- `dan-thinkpad` đang xin `join`
- `deploy-ios` chỉ chạy ở `stg`/`prd`

## Ba biến thể

| | A · Panels (lazygit) | B · Drill-down (k9s) | C · Setup tabs |
|---|---|---|---|
| Khung | Các panel xếp chồng bên trái (Projects/Configs/Commands/Machines), panel chính bên phải và dải Log | Một danh sách toàn màn hình, breadcrumb, `:` để nhảy | Bắt đầu từ `kith.yaml` gần nhất, hàng chip Config và 4 tab |
| Chọn Config | Duyệt tự do, `space` = `kith use` | Duyệt tự do, `u` = `kith use` | Chip **chính là** Active config, `[`/`]` = `kith use` |
| Việc cần người xử lý | Huy hiệu trên Config, banner trong panel secret, Log | Dải cảnh báo dưới header (luôn hiện) | Tab Activity: hộp "Needs you", Enter để xử lý ngay |
| Conflict | Màn hình xử lý chiếm panel chính | Màn hình riêng trong stack (`:conflicts`) | Ngay trong khung chi tiết bên phải |
| Grant | Chọn Machine, rồi danh sách Project/Config `[x]` | `:machines`, rồi Machine, rồi danh sách `[x]` | Ma trận Machine × Config, gom theo Member |
| Named command | Session nhúng trong TUI, nhiều tab (như Run mode của tool cũ) | Nhường hẳn terminal (`tea.ExecProcess`), xong quay về | Khung output chia đôi bên dưới danh sách lệnh |

Phím chung: `j/k` để di chuyển, `s` để giả lập đồng bộ, `v` để hiện giá trị, `e`/`a`/`d` để sửa/thêm/xóa key (mỗi thao tác là một Change cục bộ), `q` để thoát.

## Cần bạn phản hồi

1. Chọn khung nào, hay ghép (ví dụ: "header của B với tab Access của C")?
2. Chạy Named command: **nhúng trong TUI** (A/C, cần PTY như xterm của tool cũ) hay **nhường terminal** (B)?
3. Config đang xem trong TUI có nên **luôn là** Active config (C), hay xem một Config không làm đổi Active config (A/B)?
4. Việc cần xử lý (Conflict, Change bị loại, chờ xoay khóa, xin join) nên đặt ở đâu: dải luôn hiện (B) hay một hộp riêng (C)?
5. Sửa Grant theo từng máy (A/B) hay bằng ma trận (C)?
6. Ngôn ngữ giao diện: tiếng Anh (như bản nháp) hay tiếng Việt?
