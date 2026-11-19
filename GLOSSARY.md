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
