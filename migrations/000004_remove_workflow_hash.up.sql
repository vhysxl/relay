DROP INDEX idx_workflow_hash ON workflow_definitions;
DROP INDEX idx_workflow_status ON workflow_definitions;

ALTER TABLE workflow_definitions
    DROP COLUMN hash,
    DROP COLUMN status;
