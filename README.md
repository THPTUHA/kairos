# Kairos
Vấn đề: 
- Khi muốn truyền dữ liệu từ máy app android/ios này đến thiết bị android/ios khác hoặc đến web (đồng thời, lập lịch) thì phải tự xây dựng 1 server trung gian để làm điều đó.
- Training model: Khi tiến hành training 1 model 
    + Thu thập dữ liệu
    + Xử lý dữ liệu (clean, verify, fomat data)
    + Building dataset (training set, validate set, test set)
    + Training và refinement
    + Evaluation
Quá trình train này lặp đi lặp lại, cần thiết lập crawl dữ liệu và training vào 1 thời điểm thích hợp. Cần một công cụ để lập lịch, xử lý,theo dõi, báo cáo các task theo đúng quy trình trên.
- Các công việc ETL(Extract, Transform, Load) cần thực hiện định kỳ, lấy dữ liệu đâu đó và biến đổi xử lý, và lưu lại kết quả. Cần xây dựng một công cụ có thể lập lịch, xử lý, theo dõi.
- Khi mà cần thực hiện nhiều công việc định kỳ, chạy ngầm, có thể viết nhiều script cornjob để chạy.Dẫn đến phát sinh vấn đề quản lý log, chạy lại job fail, giám sát quá trình thực hiện của job. Cũng cần chia nhỏ các task để dễ kiểm soát.
- Các công việc liên quan đến devops. Cần tạo ra các task thực hiện định kỳ thu gom rác hệ thống, deploy, restart server...
- Cần gửi mail định kì, đăng quảng cáo, làm các công việc định kỳ mà không muốn tự xây dựng server hay viết script.

Giải pháp:
- Xây dựng hệ thống có khả năng:
  + Truyền data(message) từ pubscriber cho các subscriber ở một thời điểm cụ thể.
  + Xử lý được batch task, các task có rằng buộc thứ tự (song song, đồng thời), quan hệ phức tạp
  + Cung cấp khả năng xử lý job bằng script, python, sql ở local hoặc ở remate ở phía hệ thống
  + Theo dõi, thông báo quá trình thực hiện job.

HLA [https://app.diagrams.net/#G1k5ToGOQWsM0aLEsPHOlle5e0VQ891jGN]
Cấu trúc file yaml

```yaml
    collection:
        key:
        triggertime:
        timeout:
        retries:
    tasks:
        [task_name]:
            key:
            triggertime:
            executor: [path_file_code]
            timeout:
            retries:
    workflow:
        task_key_1 -> task_key_2
        [task_key_2, task_key_3] -> task_key_4 // xử lý task 2, task 3 song song, hoàn thành mới thực hiện task 4.

```

Các trạng thái của task:
none,scheduled,queue,running,success,shutdown,restarting,failed,skipped,upstream_failed,up_for_retry,up_for_reschedule,deferred,removed

## Todo
* Tìm hiểu các use case phức tạp hợn cần nhiều job và xử lý đồng thời nhiều job. (làm thêm)
* Tiến hành hiểu nghiệp vụ airflow
* Xây dựng kiến trúc 
* Tìm hiểu về cách push, pull file, quản lý file
* Cách chạy python, sql, bash.
* Orchestration container
* Tiến hành xây dựng pub/sub. (Doing) 1
* Xây dựng local.

! Architure [https://app.diagrams.net/#G1R8daJ3zLMSx6yj71d1LvGmNoEuXHV-wU]
! Tạo db,xây dựng http server cơ chế xác thực.

1. Tiến hành xây dựng pub/sub.
Xây dựng oauth
Nghiên cứu
https://github.com/distribworks/dkron
=> Chuyển mô hình master-worker sang agent + kv database

- Xác thực bằng google
- Cố gắng hoàn thành pub/sub.
- Tạo log debug

Xây dựng song song realtime-messaging server
https://github.com/centrifugal/centrifugo

## Tham khảo
https://www.reddit.com/r/ProgrammingLanguages/comments/n3yrra/advicebest_practicearhitecture_pattern_for/
https://github.com/alist-org/alist
https://github.com/mjpclab/go-http-file-server
https://github.com/appleboy/gorush

