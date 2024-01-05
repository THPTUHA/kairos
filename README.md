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
- Khi mà cần thực hiện nhiều công việc định kỳ, chạy ngầm, có thể viết nhiều script corntask để chạy.Dẫn đến phát sinh vấn đề quản lý log, chạy lại task fail, giám sát quá trình thực hiện của task. Cũng cần chia nhỏ các task để dễ kiểm soát.
- Các công việc liên quan đến devops. Cần tạo ra các task thực hiện định kỳ thu gom rác hệ thống, deploy, restart server...
- Cần gửi mail định kì, đăng quảng cáo, làm các công việc định kỳ mà không muốn tự xây dựng server hay viết script.

Giải pháp:
- Xây dựng hệ thống có khả năng:
  + Truyền data(message) từ pubscriber cho các subscriber ở một thời điểm cụ thể.
  + Xử lý được batch task, các task có rằng buộc thứ tự (song song, đồng thời), quan hệ phức tạp
  + Cung cấp khả năng xử lý task bằng script, python, sql ở local hoặc ở remate ở phía hệ thống
  + Theo dõi, thông báo quá trình thực hiện task.

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
* Tìm hiểu các use case phức tạp hợn cần nhiều task và xử lý đồng thời nhiều task. (làm thêm)
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

Hoàn thành hơn hoàn hảo
Simple hoàn thành => upgrade

## Tham khảo
https://www.reddit.com/r/ProgrammingLanguages/comments/n3yrra/advicebest_practicearhitecture_pattern_for/
https://github.com/alist-org/alist
https://github.com/mjpclab/go-http-file-server
https://github.com/appleboy/gorush


SET UP DB
createdb kairos
grant all privileges on database kairos to kairos;

kairosdeamon:
nhận task, chạy task local và trả kết quả về cho server.

watcher:
Lấy task và lập lịch
Nhận thông tin từ nats để cập nhật trạng thái task.

yum install epel-release
yum install redis -y
systemctl start redis.service
systemctl enable redis

sudo vi /etc/redis.conf
bind 0.0.0.0

redis-cli --cluster create 127.0.0.1:7000 127.0.0.1:7001 \
127.0.0.1:7002 127.0.0.1:7003 127.0.0.1:7004 127.0.0.1:7005 \
--cluster-replicas 1

  sudo ufw allow 7000
  sudo ufw reload
  sudo firewall-cmd --zone=public --add-port=7000/tcp --permanent
  sudo firewall-cmd --reload

redis-cli --cluster create 61.28.230.61:7000  135.181.207.194:7001 \
135.181.207.194:7000  103.173.254.32:7001 103.173.254.32:7000  61.28.230.61:7001 \
--cluster-replicas 1

Dấu , cuối json

protoc  --go_out=.  --go_opt=paths=source_relative \
			--go-grpc_out=server/deliverer/internal/controlpb \
			--go-vtproto_out=server/deliverer/internal/controlpb/ --plugin protoc-gen-go-vtproto=${GOPATH}/bin/protoc-gen-go-vtproto \
			--go-vtproto_opt=features=marshal+unmarshal+size \
			server/deliverer/internal/controlpb/control.proto

Pubsub:
  
kubectl exec -i postgres-5dfbf7c866-4tstv -- psql -U kairos -d kairos < /server/storage/migration/000001_kairos.up.sql

kubectl get pods --all-namespaces | awk '$4=="Evicted" {print "kubectl delete pod --namespace=" $1 " " $2}' | sh


/var/lib/pgsql/<version>/data/postgresql.conf
Sử file postgresql.conf thành listen_addresses = '*'
và pg_hba.conf thành host    all             all             0.0.0.0/0            md5
psql -h 61.28.230.61 -U kairos -d kairos -f server/storage/migration/000001_kairos.up.sql

psql -h 103.173.254.32 -U kairos -d kairos -f server/storage/migration/000001_kairos.up.sql
kubectl create configmap httpserver-config --from-file=httpserver.yaml -n kairos
kubectl create configmap runner-config --from-file=runner.yaml -n kairos
kubectl create configmap deliverer-config --from-file=deliverer.yaml -n kairos
psql -h 61.28.230.61 -U kairos -d kairos -f diagram.sql
psql -h 103.173.254.32 -U kairos -d kairos -f diagram.sql

helm install my-nginx-ingress ingress-nginx/ingress-nginx 

kubectl delete configmap httpserver-config -n kairos
kubectl delete configmap runner-config -n kairos

sudo firewall-cmd --zone=public --add-port=5432/tcp --permanent
sudo firewall-cmd --reload
sudo setenforce 0

sudo dscacheutil -flushcache
sudo killall -HUP mDNSResponder

sudo firewall-cmd --permanent --zone=public --add-icmp-block=echo-request
sudo firewall-cmd --reload
