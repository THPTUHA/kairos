CREATE TABLE IF NOT EXISTS users (
    id bigserial PRIMARY KEY,
    username VARCHAR(255) UNIQUE,
    full_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE,
    avatar VARCHAR(255),
    secret_key VARCHAR(255) UNIQUE,
    api_key VARCHAR(255) UNIQUE
);

CREATE TABLE IF NOT EXISTS workflows (
    id bigserial PRIMARY KEY,
    namespace VARCHAR(255) NOT NULl,
    name VARCHAR(255) NOT NULL,
    status INT NOT NULL,
    version VARCHAR(255) NOT NULL,
    raw_data TEXT,
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    user_id INT NOT NULl,
    CONSTRAINT fk_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS workflow_records (
    id bigserial PRIMARY KEY,
    workflow_id BIGINT NOT NULL,
    record TEXT,
    created_at BIGINT NOT NULL,
    status INT NOT NULL,
    deliver_err TEXT,
    is_recovered BOOLEAN DEFAULT FALSE,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tasks (
    id bigserial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    deps VARCHAR(800),
    schedule VARCHAR(255),
    timezone VARCHAR(255),
    clients VARCHAR(800),
    retries INT DEFAULT 0,
    executor VARCHAR(255) NOT NULL,
    workflow_id bigserial NOT NULL,
    status INT NOT NULL,
    payload TEXT,
    expires_at VARCHAR(255) NOT NULL,
    wait VARCHAR(255),
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS vars (
    id bigserial PRIMARY KEY,
    key VARCHAR(255) NOT NULL,
    value VARCHAR(255) NOT NULL,
    workflow_id bigserial NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS channels (
    id bigserial PRIMARY KEY,
    user_id INT NOT NULl,
    name VARCHAR(255) NOT NULL,
    created_at BIGINT NOT NUll,
    CONSTRAINT fk_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS brokers (
    id bigserial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    standard_name VARCHAR(255) NOT NULL,
    listens VARCHAR(800) NOT NULL,
    flows  TEXT,
    workflow_id bigserial NOT NULL,
    status INT NOT NULL,
    clients VARCHAR(255) DEFAULT '' NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_summaries (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    task_id     INT NOT NULL,
    attemp      INT NOT NULL,
	success_count INT NOT NULL,
	error_count   INT NOT NULL,
	last_success  INT NOT NULL,
	last_error    INT NOT NULL,
    created_at BIGINT NOT NUll,
    CONSTRAINT fk_task FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_records (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    output TEXT,
    task_id   INT NOT NULL,
	started_at  INT NOT NULL,
	finished_at  INT NOT NULL,
    client_id BIGINT NOT NUll,
    created_at BIGINT NOT NUll,
    CONSTRAINT fk_task FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT fk_client FOREIGN KEY(client_id) REFERENCES clients(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS broker_records (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    input TEXT,
    output TEXT,
    broker_id BIGINT NOT NUll,
    created_at BIGINT NOT NUll,
    CONSTRAINT fk_broker FOREIGN KEY(broker_id) REFERENCES brokers(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_flows (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    sender_id INT NOT NULL,
    sender_type INT NOT NULL,
    sender_name VARCHAR(255) DEFAULT '',
    receiver_id INT NOT NULL,
    receiver_type INT NOT NULL,
    receiver_name VARCHAR(255) DEFAULT '',
    workflow_id bigserial NOT NULL,
    message TEXT,
    attemp INT DEFAULT 0,
    created_at BIGINT NOT NUll,
    flow INT NOT NULL,
    deliver_id INT NOT NULL,
    elapsed_time BIGINT NOT NULL,
    request_size INT NOT NULL,
    response_size INT NOT NULL,
    cmd INT NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS clients (
    id bigserial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    user_id INT NOT NULL,
    active_since BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT fk_user_id FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);


CREATE TABLE IF NOT EXISTS certificates (
    id bigserial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    user_id BIGINT NOT NULL,
    api_key VARCHAR(255) NOT NULL,
    secret_key VARCHAR(255) NOT NULL,
    expire_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL,
    CONSTRAINT fk_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS channel_permissions (
    id bigserial PRIMARY KEY,
    cert_id BIGINT NOT NULL,
    role INT NOT NULL,
    channel_id BIGINT NOT NULL,
    CONSTRAINT fk_cert FOREIGN KEY(cert_id) REFERENCES certificates(id) ON DELETE CASCADE,
    CONSTRAINT fk_channel FOREIGN KEY(channel_id) REFERENCES channels(id) ON DELETE CASCADE
);
