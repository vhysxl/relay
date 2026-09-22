DROP INDEX idx_step_exec_queue ON step_executions;

ALTER TABLE step_executions
    DROP COLUMN next_retry_at,
    DROP COLUMN executed_at,
    DROP COLUMN is_compensation;
