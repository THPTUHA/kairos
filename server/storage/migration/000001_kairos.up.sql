CREATE TABLE IF NOT EXISTS users (
    id bigserial PRIMARY KEY,
    username VARCHAR(255) UNIQUE,
    full_name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE,
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

CREATE TABLE IF NOT EXISTS tasks (
    id bigserial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    deps VARCHAR(800),
    schedule VARCHAR(255),
    timezone VARCHAR(255),
    clients VARCHAR(800),
    retries INT DEFAULT 0,
    executor VARCHAR(255) NOT NULL,
    duration VARCHAR(255) NOT NULL,
    workflow_id bigserial NOT NULL,
    status INT NOT NULL,
    payload TEXT,
    expires_at VARCHAR(255) NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS vars (
    id bigserial PRIMARY KEY,
    key VARCHAR(255) NOT NULL,
    value VARCHAR(255) NOT NULL,
    workflow_id bigserial NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS brokers (
    id bigserial PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    listens VARCHAR(800) NOT NULL,
    flows  TEXT,
    workflow_id bigserial NOT NULL,
    status INT NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_records (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    input TEXT,
    output TEXT,
    task_id     INT NOT NULL,
    attemp      INT NOT NULL,
	success_count INT NOT NULL,
	error_count   INT NOT NULL,
	started_at    INT NOT NULL,
	finished_at   INT NOT NULL,
	last_success  INT NOT NULL,
	last_error    INT NOT NULL,
    created_at BIGINT NOT NUll,
    CONSTRAINT fk_task FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_flow (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    sender_id INT NOT NULL,
    sender_type INT NOT NULL,
    receiver_id INT NOT NULL,
    receiver_type INT NOT NULL,
    workflow_id bigserial NOT NULL,
    message TEXT,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS clients (
    id bigserial PRIMARY KEY,
    kairos_name VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    user_id INT NOT NULL,
    CONSTRAINT fk_user_id FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);