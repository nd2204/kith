# Machine là recipient, quyền nằm trong dữ liệu kith có chữ ký Admin

kith không có server trung tâm. Vì vậy, quyền đọc một Config được thực thi bằng mã hóa: data key của Config được bọc tới public key của từng **Machine** có Grant. Danh sách thành viên và Grant của mỗi Project là dữ liệu của kith, được nhân bản và chỉ hợp lệ khi có chữ ký của một Admin. Chúng tôi không dựa vào policy tailnet (ACL, grants, CapMap) vì ba lý do: tag trên tailnet xóa danh tính user, Headscale không có Tailnet Lock, và ai sửa được policy tailnet thì cấp được quyền. Lớp mạng chỉ lo vận chuyển; `WhoIs` chỉ dùng để kiểm tra thêm khi máy xin vào Project.

## Considered Options

- **Khóa theo người** (mỗi Member một khóa, chép giữa các máy): bị loại, vì phải chép private key, và mất một máy là phải xoay khóa cho mọi máy của người đó.
- **Quyền khai báo trong policy tailnet**: bị loại vì các lý do ở trên, và vì không chạy được trên LAN thuần.
- **Machine nào giữ Config cũng được cấp tiếp** (kiểu capability): bị loại, vì quyền sẽ lan ra không kiểm soát được.

## Consequences

- Mỗi Project là một phạm vi tin cậy riêng. Chỉ Machine có Grant mới nhận ciphertext của Config, và tên key cũng được mã hóa.
- Cấp quyền và bọc khóa là hai bước tách rời. Admin ký lệnh, còn Machine nào đang giữ Config thì tự bọc hoặc xoay data key. Nhờ vậy Admin không cần giữ secret.
- Thu hồi quyền không lấy lại được giá trị mà máy đó đã giữ. kith xoay data key và gắn cờ "cần xoay secret", nhưng không ép.
