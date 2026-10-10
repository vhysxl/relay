DROP INDEX idx_workflow_hash ON workflow_definitions;

ALTER TABLE workflow_definitions
    DROP COLUMN hash;

UPDATE workflow_definitions SET status = 'ACTIVE' WHERE status = 'PENDING';
