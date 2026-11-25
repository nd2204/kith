# kith

Công cụ quản lý môi trường và secret cho dev, nhân bản giữa các máy tin cậy trong một mạng riêng thay vì lưu ở server trung tâm.

## Language

**Project**:
Một bộ secret thuộc về một sản phẩm hoặc khách hàng, gom mọi Environment của nó.
_Avoid_: Profile, workspace, app

**Environment**:
Một giai đoạn triển khai của Project, ví dụ dev, staging, prod.
_Avoid_: Stage, flavor

**Config**:
Tập secret cụ thể của một Project tại một Environment; đơn vị được cấp quyền và được nhân bản.
_Avoid_: Env file, secrets file

**Branch config**:
Config kế thừa từ một Config gốc cùng Environment và chỉ ghi đè vài giá trị, ví dụ `dev_alice` kế thừa `dev`.
_Avoid_: Override, personal config

**Setup**:
File commit trong repo gắn một thư mục với Project và Config mặc định, kèm các Named command của thư mục đó.
_Avoid_: Target, manifest, link

**Named command**:
Một dòng lệnh shell có tên khai báo trong Setup, chạy với secret của Config được inject vào môi trường.
_Avoid_: Action, script, task

**Mount**:
File tạm chứa secret của một Config, do `run` tạo cho một lệnh khi được yêu cầu và xóa khi lệnh kết thúc; dành cho công cụ đọc secret từ file thay vì từ môi trường.
_Avoid_: Env file, dotenv, temp file

**Secret file**:
File chứa giá trị của một key, được ghi ra đường dẫn mà Named command khai báo và chỉ tồn tại trong lúc lệnh đó chạy; dành cho công cụ cần secret ở một đường dẫn cố định.
_Avoid_: Mount, keyfile, materialized file

**Machine**:
Một máy tin cậy chạy kith, có cặp khóa riêng; là chủ thể được cấp quyền và là recipient của Config.
_Avoid_: Device, node, peer

**Agent**:
Tiến trình nền của kith chạy trong phiên đăng nhập của user trên một Machine; là phần duy nhất của kith nhân bản Change với các Machine khác.
_Avoid_: Daemon, service, server

**Recovery machine**:
Machine có khóa được cất offline và không chạy kith thường xuyên, dùng để khôi phục quyền Admin hoặc giải mã bản sao lưu.
_Avoid_: Cold machine, recovery key, backup key

**Member**:
Một người, dùng làm nhãn gom các Machine của họ; không có khóa riêng.
_Avoid_: User, account

**Grant**:
Quyền đọc và ghi của một Machine với một Config, nghĩa là Config được mã hóa tới Machine đó; Grant một Branch config bao gồm quyền đọc Config gốc của nó.
_Avoid_: Share, access, permission

**Admin**:
Vai trò của một Member trong một Project, cho phép ký các thay đổi về thành viên và Grant của Project đó.
_Avoid_: Owner, manager

**Invite**:
Mã có thời hạn do Admin tạo, cho một máy mới tìm được Machine của Admin và gửi yêu cầu vào Project; tự nó không cấp quyền gì.
_Avoid_: Token, invite link, join code

**Change**:
Một thay đổi có chữ ký của một Machine, hoặc trên các key của một Config, hoặc trên thành viên và Grant của một Project; là đơn vị được nhân bản và gộp.
_Avoid_: Commit, revision, version, event

**Conflict**:
Tình trạng hai Change đồng thời đặt hai giá trị khác nhau cho cùng một key của một Config; Config vẫn đọc được, nhưng cần người chọn giá trị đúng.
_Avoid_: Collision, divergence

**Format floor**:
Format thấp nhất mà một Project chấp nhận cho Change mới; chỉ Admin nâng được, và không bao giờ hạ.
_Avoid_: Min version, schema version

**Active config**:
Config mà một Machine đang dùng cho một Project khi lệnh không chỉ định Config khác; mặc định là Config ghi trong Setup.
_Avoid_: Current config, selected profile

**Rollback**:
Một Change đặt lại một key, hoặc cả Config, về giá trị chúng có tại một Change cũ hơn; lịch sử không bị xóa hay viết lại.
_Avoid_: Revert, undo, restore

**Archive**:
Trạng thái của một Config hoặc Project đã ngừng dùng: không còn chạy hay ghi được, nhưng không mất gì và bật lại được.
_Avoid_: Delete, remove
