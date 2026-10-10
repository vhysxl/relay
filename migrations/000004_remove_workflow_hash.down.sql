ALTER TABLE workflow_definitions
    ADD COLUMN hash VARCHAR(64) NOT NULL DEFAULT '' AFTER status;

CREATE INDEX idx_workflow_hash ON workflow_definitions (hash);
