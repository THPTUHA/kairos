-- INSERT INTO users (username, full_name, email, avatar, secret_key, api_key)
-- VALUES ('nghiabdao', 'nghiabadao', 'nghiabadao@gmail.com', 'default_avatar.jpg', 'user_token', 'user_api_key');

DELETE FROM clients ;

INSERT INTO clients (id, name, user_id, active_since, created_at)
VALUES 
  (1,'client1', 1, 1702577960 , 1702577960),
  (2,'client3', 1, 1702577960 , 1702577960),
  (4,'client2', 1, 1702577960 , 1702577960);
