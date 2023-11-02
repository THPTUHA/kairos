CREATE TABLE IF NOT EXISTS users (
    id bigserial PRIMARY KEY,
    username VARCHAR(255) UNIQUE,
    full_name VARCHAR(255) NOT NULL,
    password VARCHAR(255),
    email VARCHAR(255) UNIQUE,
    token VARCHAR(255) UNIQUE
);

CREATE TABLE IF NOT EXISTS collections (
    id bigserial PRIMARY KEY,
    raw_data TEXT,
    user_id INT NOT NULL,
    created_at INT NOT NULL,
    updated_at INT NOT NULL,
    CONSTRAINT fk_user FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS workflows (
    id bigserial PRIMARY KEY,
    key VARCHAR(255) NOT NULL,
    status INT NOT NULL,
    collection_id INT NOT NULL,
    created_at INT NOT NULL,
    updated_at INT NOT NULL,
    CONSTRAINT fk_collection FOREIGN KEY(collection_id) REFERENCES collections(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tasks (
    id bigserial PRIMARY KEY,
    key VARCHAR(255) NOT NULL,
    schedule VARCHAR(255) NOT NULL,
    timezone VARCHAR(255) NOT NULL,
    executor VARCHAR(255) NOT NULL,
    timeout INT NOT NULL,
    retries INT DEFAULT 0,
    inputs TEXT,
    run TEXT,
    workflow_id INT NOT NULL,
    status INT NOT NULL,
    strict BOOLEAN NOT NULL,
    env_exec VARCHAR(255) NOT NULL,
    created_at INT NOT NULL,
    updated_at INT NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_records (
    id bigserial PRIMARY KEY,
    status INT NOT NULL,
    output TEXT,
    task_id INT NOT NULL,
    attemp      INT NOT NULL,
	success_count INT NOT NULL,
	error_count   INT NOT NULL,
	started_at    INT NOT NULL,
	finished_at   INT NOT NULL,
	last_success  INT NOT NULL,
	last_error    INT NOT NULL,
    CONSTRAINT fk_task FOREIGN KEY(task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS graph_tasks (
    id bigserial PRIMARY KEY,
    task_parent_id INT NOT NULL,
    task_child_id INT NOT NULL,
    workflow_id INT NOT NULL,
    CONSTRAINT fk_workflow FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE,
    CONSTRAINT fk_task_parent FOREIGN KEY(task_parent_id) REFERENCES tasks(id) ON DELETE CASCADE,
    CONSTRAINT task_child_id FOREIGN KEY(task_child_id) REFERENCES tasks(id) ON DELETE CASCADE
);