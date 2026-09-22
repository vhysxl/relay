CREATE TABLE step_definitions (
    id              VARCHAR(100) NOT NULL,
    workflow_id     VARCHAR(36) NOT NULL,
    name            VARCHAR(255) NULL,
    type            VARCHAR(50) NOT NULL,
    config          JSON NOT NULL,
    depends_on      JSON NULL,
    timeout         INT NOT NULL DEFAULT 0,
    retry           JSON NULL,
    compensation    JSON NULL,
    irreversible    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
                    ON UPDATE CURRENT_TIMESTAMP,

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
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
                    ON UPDATE CURRENT_TIMESTAMP,

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
    type                    VARCHAR(50) NOT NULL,
    config                  JSON NOT NULL,
    depends_on              JSON NULL,
    status                  VARCHAR(50) NOT NULL,
    output                  JSON NULL,
    error_message           TEXT NULL,
    attempt                 INT NOT NULL DEFAULT 0,
    max_attempts            INT NOT NULL DEFAULT 1,
    timeout_seconds         INT NOT NULL DEFAULT 0,
    backoff_seconds         INT NOT NULL DEFAULT 0,
    multiplier              DECIMAL(5,2) NOT NULL DEFAULT 1.00,
    max_backoff_seconds     INT NOT NULL DEFAULT 0,
    locked_by               VARCHAR(255) NULL,
    locked_until            TIMESTAMP NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
                            ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    INDEX idx_step_exec_wf_exec (workflow_execution_id),
    INDEX idx_step_exec_status (status),
    INDEX idx_step_exec_locked (locked_until, status),
    CONSTRAINT fk_step_executions_wf_exec
        FOREIGN KEY (workflow_execution_id) REFERENCES workflow_executions(id)
        ON DELETE CASCADE
);
