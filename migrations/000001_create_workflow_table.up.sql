CREATE TABLE workflow_definitions (
    id          VARCHAR(36) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    version     INT NOT NULL DEFAULT 1,
    status      VARCHAR(50) NOT NULL,
    hash        VARCHAR(64) NOT NULL,
    context     JSON NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
                ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    UNIQUE KEY uq_workflow_name_version (name, version),
    INDEX idx_workflow_status (status),
    INDEX idx_workflow_hash (hash)
);