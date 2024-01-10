-- DELETE FROM users ;
-- INSERT INTO users (id,username, full_name, email, avatar, secret_key, api_key)
-- VALUES (1,'nghiabdao', 'nghiabadao', 'nghiabadao@gmail.com', 'https://avatars.githubusercontent.com/u/82744969?v=4', 'user_token', 'user_api_key');

-- DELETE FROM clients ;

-- INSERT INTO clients (id, name, user_id, active_since, created_at)
-- VALUES 
--   (1,'client1', 1, 1702577960 , 1702577960),
--   (2,'client3', 1, 1702577960 , 1702577960),
--   (4,'client2', 1, 1702577960 , 1702577960);
-- select * from users;

-- select * from certificates

-- delete from workflows;

-- select * from message_flows where parent='part-239-1704554976' limit 10;
-- select * from message_flows where flow=0 and task_name='notification';
-- select * from message_flows where "group"='kairos-239-1704646761-0';
-- select * from message_flows where start = true;

-- \dt
-- drop table schema_migrations;
-- drop table honors;
-- drop table triggers;
-- drop table message_flows;
-- drop table honors;
-- select id, trigger_id, sender_id, receiver_id from message_flows where  trigger_id = 5 and start = true;
-- select * from triggers where status = 2;
-- select *  from message_flows where deliver_id =1704800425;
-- select * from message_flows where status =-99;
-- delete from triggers;
-- select * from message_flows order by id desc limit 1;
-- select * from message_flows where deliver_id = 
-- drop table workflow_records;
-- select * from workflow_records where deliver_id =1704800418;


-- delete from workflow_records;
-- delete from message_flows;

-- \dt
-- select * from message_flows order by id desc limit 2;
INSERT INTO honors(key, title, date, content, status)VALUES
('first', 'Ngày Đầu Đi Học', 'September 2019', 'Năm 2019, tôi bắt đầu hành trình đại học tại Học Viện Công Nghệ Bưu Chính Viễn Thông, với niềm đam mê và mong đợi lớn lao về ngành công nghệ thông tin. Đó là một ngày đầy nắng và hứng khởi, nơi tôi bắt đầu chương mới trong cuộc đời - một chương với những thách thức mới, những kiến thức mới và một môi trường học tập mới.</br> </br>
Buổi lễ khai giảng diễn ra trong không khí trang trọng và phấn khích. Những bảng hướng dẫn và bảng thông tin nổi bật khắp nơi, mời gọi chúng tôi khám phá thế giới mới mà chúng tôi sắp trở thành một phần.</br></br>
Ngày đầu tiên tại Học Viện Công Nghệ Bưu Chính Viễn Thông không chỉ là tìm hiểu về trường mà còn là cơ hội để làm quen với những người bạn mới. Trong cuộc họp lớp, tôi nhận thức được sự đa dạng và sự hỗ trợ từ đồng học cùng lớp, một lớp học mà tôi tin chắc sẽ chia sẻ những chặng đường đầy thách thức và thành công cùng tôi. </br></br>
Ngày đầu tiên tại Học Viện Công Nghệ Bưu Chính Viễn Thông là một trải nghiệm khó quên, là bước đầu tiên trong hành trình chinh phục kiến thức và định hình tương lai trong ngành công nghệ thông tin. Tôi háo hức đón nhận những thách thức mới và rất mong đợi những bài giảng, bài học mới. </br></br>
Chặng đường học tập tại Học Viện Công Nghệ Bưu Chính Viễn Thông là hành trình học hỏi, rèn luyện trang bị kiến thức để bước vào đời.', 
1),
('graduation', 'LỜI CẢM ƠN', 'January 2024', 'Lời đầu tiên, em xin chân thành cảm ơn các thầy, cô trong Khoa Công nghệ Thông tin và toàn thể cán bộ của Học viện Công nghệ Bưu chính Viễn thông đã quan tâm, dạy bảo và truyền đạt kiến thức cần thiết và bổ ích cho em trong suốt thời gian học tập tại học viện, những kiến thức này không chỉ là tiền đề, hành trang để em thực hiện đồ án này mà còn phục vụ cho công việc trong tương lai. Để hoàn thành tốt quá trình học tập em đã được các thầy cô chỉ bảo không chỉ ở kiến thức chuyên môn, chuyên ngành, mà đó còn là sự chia sẻ, những kinh nghiệm trong cuộc sống.</br></br>
Em xin gửi lời biết ơn sâu sắc tới Thầy giáo hướng dẫn của em –TS. Đặng Ngọc Hùng, người đã tận tình chỉ bảo, định hướng cho em trong suốt quá trình học tập và thực hiện đồ án. Thầy luôn theo dõi giúp đỡ, động viên và tin tưởng để em hoàn thành một cách tốt nhất đồ án này.</br></br>
Cuối cùng, em xin cảm ơn gia đình, bạn bè, những người đã luôn ở bên cạnh, quan tâm, giúp đỡ, động viên để bản thân có thể hoàn thành được đồ án này. Với kiến thức hiểu biết của em còn hạn chế nên đồ án không tránh khỏi thiếu sót, em rất mong nhận được sự góp ý từ các thầy cô và các bạn để đồ án của em được hoàn thiện hơn.</br></br>
Em xin chân thành cảm ơn!', 0);
