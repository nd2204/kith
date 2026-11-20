# Khôi phục quyền Admin và sao lưu bằng Recovery machine, không thêm loại khóa mới

Mất hết máy của mọi Admin là mất quyền quản trị Project. kith không có server để làm cửa sau. Vì vậy, cơ chế khôi phục là một **Recovery machine**: một Machine bình thường của một Member Admin, nhưng khóa của nó được xuất ra để cất offline và không chạy kith thường xuyên. Recovery machine không cần luật mới khi gộp. Quyền ký Admin của nó đến từ vai trò của Member, như mọi Machine khác. Nó cũng có thể nhận Grant, vì bọc data key chỉ cần public key. Nhờ vậy, file do `kith backup export` tạo ra (các Change đã mã hóa cùng các data key đã bọc) vẫn giải mã được khi mọi bản sao của Config đều mất. Khi khôi phục, `kith recover` chỉ nạp khóa vào bộ nhớ, ký một Change duyệt máy hiện tại vào Member Admin, rồi bỏ khóa đi. Khóa khôi phục không bao giờ thành khóa của một máy chạy thường xuyên.

## Considered Options

- **Không có cơ chế khôi phục** (quyết định ban đầu ở ticket cấp quyền): vẫn là lối thoát cuối cùng. Các máy còn Grant tạo một Project mới rồi `import`, nhưng mọi thành viên và Grant phải dựng lại từ đầu.
- **Khóa khôi phục riêng của Project** (tách khỏi Machine): bị loại, vì thêm một loại chủ thể có quyền ký. Log Project phải học thêm luật cho chủ thể này.
- **M-of-N Member cùng ký để phong Admin mới**: bị loại khỏi v1, vì phải thêm luật ngưỡng vào phần gộp và xét quyền theo quan hệ nhân quả.

## Consequences

- Recovery machine là tùy chọn. TUI cảnh báo khi Project chỉ có một Machine có quyền Admin, và Recovery machine được tính vào số này.
- Cảnh báo "Config chỉ có một bản sao" chỉ đếm Machine thường đang giữ ciphertext. Recovery machine và các file sao lưu không được tính.
- Mỗi lần xoay data key, máy đang online tự bọc khóa mới cho Recovery machine nếu nó có Grant. Một file sao lưu cũ chỉ giải mã được tới epoch mà file đó chứa.
- Ai giữ khóa của Recovery machine thì có quyền Admin. Lộ khóa thì gỡ Recovery machine đó với lý do `lost`, rồi tạo cái mới.
