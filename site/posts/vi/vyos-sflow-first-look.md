---
title: Ai đang ăn hết traffic VyOS của tôi? Công cụ nhỏ này chỉ ra ngay 😭
date: 2026-12-04
summary: Mạng văn phòng cứ tới giờ cao điểm là chậm như rùa, mà chẳng ai biết ai đang ăn băng thông? Một binary là xong: băng thông, top talkers và bản ghi flow, cho bất cứ thiết bị nào xuất được sFlow, xem bằng trình duyệt hoặc qua SSH. Chạy xong trong 30 giây.
---

Nói nỗi đau trước 👇

Con VyOS ở cổng ra của văn phòng chạy đã lâu lắm rồi.
Cứ tới giờ cao điểm ngày làm việc là mạng chậm hẳn.
Ai đang ăn băng thông? Không biết.
Máy nào, đang nói chuyện với server nào? Cũng không biết.
Toàn đoán mò. Câu trả lời lúc nào cũng là "khởi động lại thử xem" 🙂

VyOS xuất được mẫu sFlow.
Hầu hết switch, router hãng khác và server cũng xuất được.

Nhưng rồi sao nữa? Chẳng có gì để xem cả ❌
Mấy công cụ quen thuộc thì đòi một server riêng, hoặc database với cả một bộ dashboard.
Quá nặng, chỉ để xem traffic của chính mình.

Nên tôi tự viết một công cụ nhỏ. Tên là vyos-sflow ✨
Chỉ một binary.
Chạy ngay trên router,
hoặc để trên bất kỳ máy Linux nào và cho mọi thiết bị gửi sFlow về đó.

## 📦 Cài đặt chỉ 3 bước

Đầu tiên, bật sFlow trong VyOS (bỏ qua nếu nó đã gửi tới 127.0.0.1:6343):

```
configure
set system sflow interface eth0
set system sflow server 127.0.0.1 port 6343
commit
save
exit
```

Thiết bị không phải VyOS? Cũng vậy thôi: trỏ xuất sFlow của thiết bị đó về máy này, cổng 6343, rồi làm tiếp bên dưới.

Rồi tải về và chạy:

```
curl -fLO https://github.com/githubflyideas/vyos-sflow/releases/download/v1.3.3/vyos-sflow-1.3.3-linux-amd64.tar.gz
tar -xzf vyos-sflow-1.3.3-linux-amd64.tar.gz
cd vyos-sflow-1.3.3-linux-amd64 && ./vyos-sflow serve
```

Mở `http://ROUTER_IP:8066` trong trình duyệt, đăng nhập bằng admin / tryvyossflow66, rồi đổi mật khẩu.
Chỉ vậy thôi. Biểu đồ bắt đầu có dữ liệu.

![Trang tổng quan](assets/img/screenshots/vyos/web-overview.png)

## 👀 Bạn nhận được gì

✅ Băng thông hiện tại, và đường cong của 48 giờ gần nhất

✅ Top talkers: client nào và server nào dùng nhiều traffic nhất, xếp hạng rõ ràng

✅ Traffic của interface từ bộ đếm của chính thiết bị, đặt cạnh ước tính từ mẫu, để bạn tự xem số liệu có khớp không

✅ Các bản ghi flow phía sau biểu đồ — bấm vào một đỉnh nhọn và tìm ra đó là ai

✅ Nếu biểu đồ trống, trang Sources sẽ nói cho bạn vì sao. Không cần đoán nữa

![Top talkers](assets/img/screenshots/vyos/web-top.png)

## 🖥️ Không có trình duyệt? Dùng terminal

SSH vào router và chạy một lệnh:

```
./vyos-sflow tui
```

Vẫn là bức tranh đó, ngay trong terminal. Nhấn q để thoát.
Không cài thêm gì cả, rất tiện khi chỉ muốn xem nhanh.

Thêm vài điểm nhỏ: giao diện web có 15 ngôn ngữ và 5 theme, từ sáng tới tối.

## ⚠️ Những giới hạn nói thật

- Chỉ sFlow. Không có NetFlow, không có IPFIX
- Lịch sử là 48 giờ. Nó trả lời "đang xảy ra chuyện gì" và "vừa xảy ra chuyện gì", không phải báo cáo dài hạn
- Chỉ Linux amd64. Đủ cho VyOS và server Linux bình thường, nhưng không có bản Windows hay macOS

Nếu ba điều này ổn với bạn, nó làm việc rất tốt.

## 🔗 Lấy ở đâu

Miễn phí và mã nguồn mở, Apache-2.0.
Tìm trên GitHub với vyos-sflow, hoặc dùng link tải trên site này.
Nếu không thích, xóa thư mục là xong — router tự nó không bị đụng vào.

Nếu VyOS đang làm cổng ra của văn phòng hay quán net, thử xem nhé.
Cảm giác "traffic của mình đi đâu mất rồi" sẽ biến mất.

Mọi người dùng gì để xem traffic ở cổng ra? Kể ở dưới nhé 👇

#VyOS #sFlow #opensource #networking #sysadmin
