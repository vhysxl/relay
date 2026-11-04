DROP TABLE IF EXISTS step_execution_attempts;

DROP INDEX idx_step_exec_routing_claim ON step_executions;

ALTER TABLE step_executions
    DROP COLUMN activity_name,
    DROP COLUMN task_queue;

