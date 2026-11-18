CREATE TABLE workflow_definitions (
    id          VARCHAR(36) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    version     INT NOT NULL DEFAULT 1,
    context     JSON NULL,
    created_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (id),
    UNIQUE KEY uq_workflow_name_version (name, version)
);
CREATE TABLE step_definitions (
    id              VARCHAR(100) NOT NULL,
    workflow_id     VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NULL,
    queue VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    activity VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    input JSON NOT NULL,
    depends_on      JSON NULL,
    timeout         INT NOT NULL DEFAULT 0,
    retry           JSON NULL,
    compensation    JSON NULL,
    irreversible    BOOLEAN NOT NULL DEFAULT FALSE,
    `condition` JSON NULL,
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                    ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (workflow_id, id),
    INDEX idx_step_workflow_id (workflow_id),
    CONSTRAINT fk_step_definitions_workflow
        FOREIGN KEY (workflow_id) REFERENCES workflow_definitions(id)
        ON DELETE CASCADE
);

CREATE TABLE workflow_executions (
    id              VARCHAR(36) NOT NULL,
    workflow_id     VARCHAR(36) NOT NULL,
    status          VARCHAR(50) NOT NULL,
    context         JSON NULL,
    error_message   TEXT NULL,
    created_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at      TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                    ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (id),
    INDEX idx_wf_exec_workflow_id (workflow_id),
    INDEX idx_wf_exec_status (status),
    CONSTRAINT fk_workflow_executions_workflow
        FOREIGN KEY (workflow_id) REFERENCES workflow_definitions(id)
        ON DELETE CASCADE
);

CREATE TABLE step_executions (
    id                      VARCHAR(36) NOT NULL,
    workflow_execution_id   VARCHAR(36) NOT NULL,
    step_id                 VARCHAR(100) NOT NULL,
    name                    VARCHAR(255) NULL,
    queue VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    activity VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL,
    input JSON NOT NULL,
    depends_on              JSON NULL,
    status                  VARCHAR(50) NOT NULL,
    output                  JSON NULL,
    error_message           TEXT NULL,
    attempt                 INT NOT NULL DEFAULT 0,
    max_attempts            INT NOT NULL DEFAULT 1,
    timeout_seconds         INT NOT NULL,
    backoff_seconds         INT NOT NULL DEFAULT 0,
    multiplier              DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    max_backoff_seconds     INT NOT NULL DEFAULT 0,
    locked_by               VARCHAR(255) NULL,
    locked_until            TIMESTAMP(3) NULL,
    is_compensation BOOLEAN NOT NULL DEFAULT FALSE,
    `condition` JSON NULL,
    condition_evaluation JSON NULL,
    executed_at TIMESTAMP(3) NULL,
    next_retry_at TIMESTAMP(3) NULL,
    created_at              TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                            ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (id),
    INDEX idx_step_exec_wf_exec (workflow_execution_id),
    INDEX idx_step_exec_status (status),
    INDEX idx_step_exec_locked (locked_until, status),
    CONSTRAINT fk_step_executions_wf_exec
        FOREIGN KEY (workflow_execution_id) REFERENCES workflow_executions(id)
        ON DELETE CASCADE
);

CREATE TABLE workflow_execution_requests (
    workflow_name           VARCHAR(255) NOT NULL,
    idempotency_key         VARBINARY(128) NOT NULL,
    workflow_execution_id   VARCHAR(36) NOT NULL,
    input_context           LONGBLOB NOT NULL,
    response_receipt        LONGBLOB NOT NULL,
    created_at              TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (workflow_name, idempotency_key),
    UNIQUE KEY uq_wf_exec_req_execution (workflow_execution_id),
    CONSTRAINT fk_wf_exec_requests_execution
        FOREIGN KEY (workflow_execution_id) REFERENCES workflow_executions(id)
        ON DELETE CASCADE
);

CREATE TABLE step_execution_attempts (
    step_execution_id       VARCHAR(36) NOT NULL,
    attempt                 INT NOT NULL,
    service_id              VARCHAR(128) NOT NULL,
    worker_id               VARCHAR(255) NOT NULL,
    lease_token_hash        VARCHAR(64) NOT NULL,
    started_at              TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    deadline                TIMESTAMP(3) NOT NULL,
    finished_at             TIMESTAMP(3) NULL,
    outcome                 VARCHAR(50) NOT NULL,
    error_message           TEXT NULL,
    report_type             VARCHAR(50) NULL,
    report_payload_hash     VARCHAR(64) NULL,
    created_at              TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at              TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                            ON UPDATE CURRENT_TIMESTAMP(3),

    PRIMARY KEY (step_execution_id, attempt),
    INDEX idx_attempts_lease (step_execution_id, lease_token_hash),
    CONSTRAINT fk_step_execution_attempts_step_exec
        FOREIGN KEY (step_execution_id) REFERENCES step_executions(id)
        ON DELETE CASCADE
);

CREATE INDEX idx_step_exec_queue ON step_executions (status, locked_until, next_retry_at);
CREATE INDEX idx_step_exec_routing_claim ON step_executions (queue, status, locked_until, next_retry_at, is_compensation);
