# Đồng bộ qua Agent cấp user; lệnh chỉ chạm bản sao cục bộ; xác thực peer bằng mTLS từ khóa Machine

Muốn nhân bản thì phải có thứ gì đó lắng nghe kết nối đến. Vì vậy mỗi Machine chạy một **Agent**: một service cấp user (LaunchAgent trên macOS, `systemd --user` trên Linux, Scheduled Task lúc đăng nhập trên Windows), và đây là tiến trình duy nhất của kith giữ mạng. Agent chạy trong phiên của user vì khóa Machine nằm trong kho khóa của user. Các lệnh `run`, `secrets` và TUI chỉ đọc và ghi bản sao cục bộ, không bao giờ chờ mạng theo mặc định. Chúng chỉ nudge Agent qua IPC cục bộ, và IPC không mang secret. `kith sync` đồng bộ một lần ở foreground cho máy không có Agent. Hai Machine xác thực nhau bằng mTLS với chứng chỉ tự ký từ khóa Ed25519 của Machine, đối chiếu với danh sách Machine trong log Project. `WhoIs` chỉ là kiểm tra phụ.

## Considered Options

- **Chỉ đồng bộ khi chạy lệnh**: bị loại. Hai máy chỉ trao đổi được log khi cả hai cùng đang chạy lệnh, tức là gần như không bao giờ.
- **Mọi lệnh đi qua daemon**: bị loại. Khi Agent chết, `run` sẽ hỏng theo, trái nguyên tắc đọc cục bộ luôn được.
- **System service chạy bằng root**: bị loại. Nó chạy ngoài phiên của user, nên không dùng được kho khóa của user một cách tin cậy, và còn đòi quyền admin khi cài.
- **Tin danh tính tailnet (`WhoIs`) để xác thực peer**: bị loại, cùng lý do với [ADR 0001](0001-machine-la-recipient-quyen-trong-du-lieu-kith.md). mTLS từ khóa Machine cũng chạy được trên transport không mã hóa sau này.

## Consequences

- Địa chỉ của peer là trạng thái mềm cục bộ (stable NodeID, tên MagicDNS), được đổi thành IP qua LocalAPI. Địa chỉ không nằm trong log Project, vì danh tính đã được xác thực ở mTLS.
- Agent chỉ bind vào IP tailnet, ở một cổng cố định chỉnh được. Agent nhận client lạ ở đúng một endpoint duy nhất là `join`, và chỉ cho máy mang **Invite**.
- Không có relay qua máy không có Grant. Nếu các máy không cùng online thì nhân bản bị trễ. Cách khắc phục là thêm một Machine luôn bật có Grant.
