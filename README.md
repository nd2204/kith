# kith

Quản lý môi trường và secret cho dev, giống Doppler nhưng **phân tán**: secret được nhân bản giữa các máy tin cậy trong một mạng riêng (tailnet), không có server trung tâm. Viết bằng Go, TUI bằng Bubble Tea.

Trạng thái: **spec v1 xong, chờ implement**. Đọc [docs/PRD.md](docs/PRD.md) trước; quyết định khó đảo ngược nằm ở [docs/adr/](docs/adr/), thuật ngữ ở [GLOSSARY.md](GLOSSARY.md), kế hoạch chuyển vet45 ở [docs/migration-vet45.md](docs/migration-vet45.md). Lộ trình nằm ở issue có nhãn `wayfinder:map`.
