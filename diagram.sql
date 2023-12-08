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
  created_at bigint 
  updated_at bigint 
  user_id INT 
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
  workflow_id bigserial 
  status INT 
  payload TEXT
  expires_at VARCHAR(255) 
}

Table vars {
  id bigserial [primary key]
  key VARCHAR(255) 
  value VARCHAR(255) 
  workflow_id bigserial 

}

Table channels {
  id bigserial [primary key]
  user_id INT 
  name VARCHAR(255) 
  created_at bigint 
}

Table brokers {
  id bigserial [primary key]
  name VARCHAR(255) 
  standard_name VARCHAR(255) 
  listens VARCHAR(800) 
  flows TEXT
  workflow_id bigserial 
  status INT 
}

Table task_summaries {
  id bigserial [primary key]
  status INT 
  task_id INT 
  attemp INT 
  success_count INT 
  error_count INT 
  last_success INT 
  last_error INT 
  created_at bigint 
}

Table task_records {
  id bigserial [primary key]
  status INT 
  input TEXT
  output TEXT
  task_id INT 
  started_at INT 
  finished_at INT 
  created_at bigint 
}

Table message_flows {
  id bigserial [primary key]
  status INT
  sender_id INT
  sender_type INT 
  receiver_id INT 
  receiver_type INT 
  workflow_id bigserial 
  message TEXT
  attemp INT
  created_at bigint 
  flow INT 
  deliver_id INT 
  elapsed_time bigint 
  request_size INT 
  response_size INT 
}

Table clients {
  id bigserial [primary key]
  name VARCHAR(255)
  user_id INT 
  active_since bigint 
  created_at bigint
}

Table certificates {
  id bigserial [primary key]
  name VARCHAR(255) 
  user_id bigserial 
  api_key VARCHAR(255) 
  secret_key VARCHAR(255) 
  expire_at bigint 
  created_at bigint 
}

Table channel_permissions {
  id bigserial [primary key]
  cert_id bigserial 
  role INT 
  channel_id bigserial 
}

Ref: workflows.user_id > users.id 
Ref: tasks.workflow_id > workflows.id 
Ref: vars.workflow_id > workflows.id 
Ref: channels.user_id > users.id
Ref: task_summaries.task_id > tasks.id
Ref: task_records.task_id > tasks.id
Ref: message_flows.workflow_id > workflows.id
Ref: clients.user_id > users.id
Ref: certificates.user_id > users.id

Ref: channel_permissions.channel_id > channels.id
Ref: channel_permissions.cert_id > certificates.id









