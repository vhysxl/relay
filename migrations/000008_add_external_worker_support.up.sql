ALTER TABLE step_executions
    ADD COLUMN task_queue VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT 'legacy' AFTER config,
    ADD COLUMN activity_name VARCHAR(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER task_queue;

CREATE INDEX idx_step_exec_routing_claim ON step_executions (task_queue, status, locked_until, next_retry_at, is_compensation);

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

UPDATE step_executions
SET task_queue = COALESCE(NULLIF(JSON_UNQUOTE(JSON_EXTRACT(config, '$.queue')), 'null'), 'legacy')
WHERE config IS NOT NULL;

UPDATE step_executions
SET activity_name = NULLIF(JSON_UNQUOTE(JSON_EXTRACT(config, '$.activity')), 'null')
WHERE type = 'activity' AND config IS NOT NULL;

