Table users {
  id bigserial [primary key]
  username VARCHAR(255) unique
  full_name VARCHAR(255)
  email VARCHAR(255) unique
  avatar VARCHAR(255)
  secret_key VARCHAR(255) unique
  api_key VARCHAR(255) unique
  created_at timestamp
}

Table workflows {
  id bigserial [primary key]
  namespace VARCHAR(255)
  name VARCHAR(255)
  status INT
  version VARCHAR(255)
  raw_data TEXT
  created_at BIGINT
  updated_at BIGINT
  user_id INT [ref: > users.id]
}

Table workflow_records {
  id bigserial [primary key]
  workflow_id BIGINT [ref: > workflows.id]
  record TEXT
  created_at BIGINT
  status INT
  deliver_err TEXT
  is_recovered BOOLEAN
}

Table tasks {
  id bigserial [primary key]
  name VARCHAR(255)
  deps VARCHAR(800)
  schedule VARCHAR(255)
  timezone VARCHAR(255)
  clients VARCHAR(800)
  retries INT 
  executor VARCHAR(255)
  workflow_id bigserial [ref: > workflows.id]
  status INT
  payload TEXT
  expires_at VARCHAR(255)
  wait VARCHAR(255)
}

Table vars {
  id bigserial [primary key]
  key VARCHAR(255)
  value VARCHAR(255)
  workflow_id bigserial [ref: > workflows.id]
}

Table channels {
  id bigserial [primary key]
  user_id INT [ref: > users.id]
  name VARCHAR(255)
  created_at BIGINT
}

Table brokers {
  id bigserial [primary key]
  name VARCHAR(255)
  standard_name VARCHAR(255)
  listens VARCHAR(800)
  flows TEXT
  workflow_id bigserial [ref: > workflows.id]
  status INT
  clients VARCHAR(255)
}

Table task_summaries {
  id bigserial [primary key]
  status INT
  task_id INT [ref: > tasks.id]
  attemp INT
  success_count INT
  error_count INT
  last_success INT
  last_error INT
  created_at BIGINT
}

Table task_records {
  id bigserial [primary key]
  status INT
  output TEXT
  task_id INT [ref: > tasks.id]
  started_at INT
  finished_at INT
  client_id BIGINT [ref: > clients.id]
  created_at BIGINT
}

Table broker_records {
  id bigserial [primary key]
  status INT
  input TEXT
  output TEXT
  broker_id BIGINT [ref: > brokers.id]
  created_at BIGINT
}

Table message_flows {
  id bigserial [primary key]
  status INT
  sender_id INT
  sender_type INT
  sender_name VARCHAR(255) 
  receiver_id INT
  receiver_type INT
  receiver_name VARCHAR(255)
  workflow_id bigserial [ref: > workflows.id]
  message TEXT
  attemp INT
  created_at BIGINT
  flow INT
  deliver_id INT
  elapsed_time BIGINT
  request_size INT
  response_size INT
  cmd INT
}

Table clients {
  id bigserial [primary key]
  name VARCHAR(255)
  user_id INT [ref: > users.id]
  active_since BIGINT
  created_at BIGINT
}

Table certificates {
  id bigserial [primary key]
  name VARCHAR(255)
  user_id BIGINT [ref: > users.id]
  api_key VARCHAR(255)
  secret_key VARCHAR(255)
  expire_at BIGINT
  created_at BIGINT
}

Table channel_permissions {
  id bigserial [primary key]
  cert_id BIGINT [ref: > certificates.id]
  role INT
  channel_id BIGINT [ref: > channels.id]
}
