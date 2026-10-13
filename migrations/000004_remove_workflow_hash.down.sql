ALTER TABLE workflow_definitions
    ADD COLUMN status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE' AFTER version,
    ADD COLUMN hash VARCHAR(64) NOT NULL DEFAULT '' AFTER status;

CREATE INDEX idx_workflow_status ON workflow_definitions (status);
CREATE INDEX idx_workflow_hash ON workflow_definitions (hash);
