# Mỗi app là một Project; `run` đọc thẳng Setup và Setup không biến đổi key

Trong một monorepo, mỗi app là một **Project** riêng và có `kith.yaml` riêng trong thư mục của nó. `kith run` đọc thẳng file Setup đã commit, chọn file gần nhất khi đi ngược lên từ thư mục hiện tại. Setup chỉ trỏ tới Project và Config mặc định, khai báo `requires` (chỉ gồm tên key) và các Named command. Setup không lọc, không đổi tên và không đặt giá trị mặc định cho key. Giá trị dùng chung giữa các app nằm ở một Project chung và được tham chiếu bằng `${project.config.KEY}` ngay trong Config. Lý do chính là Grant: Config là đơn vị được cấp quyền. Nếu cả repo là một Project, máy chỉ chạy một app cũng nhận secret của mọi app khác.

## Considered Options

- **Cả repo là một Project, Setup của mỗi app lọc và đổi tên key** (cách của `vet45_env`, với `keys`, `from`, `default`): bị loại. Grant không thể mịn hơn mức cả repo. Ngoài ra, phép ánh xạ key sẽ nằm trong file commit, không có chữ ký và không có lịch sử như Config.
- **Một Setup ở gốc repo, chứa danh sách entry có `path`** (giống `setup:` dạng list của Doppler): bị loại. Danh sách `path` phải được giữ đồng bộ với cây thư mục, còn Setup nằm cạnh app thì dời đi theo app.
- **`setup` ghi ánh xạ thư mục vào trạng thái máy, `run` đọc trạng thái đó** (giống `~/.doppler`): bị loại, vì dời thư mục là mất cấu hình. Trạng thái máy chỉ giữ Active config, lưu theo khóa Project chứ không theo đường dẫn.

## Consequences

- Repo nhiều app sẽ có nhiều Project, cộng thêm một Project chung cho các giá trị dùng chung. Khi chuyển từ `vet45_env`, mỗi khai báo `from` trở thành một tham chiếu trong Config.
- Muốn đọc một tham chiếu chéo Project thì máy phải có Grant tới Config đích.
- Setup lồng nhau không kế thừa nhau, nên chỉ cần đọc một file là biết lệnh nào sẽ chạy.
