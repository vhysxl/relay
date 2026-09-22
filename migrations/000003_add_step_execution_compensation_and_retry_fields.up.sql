ALTER TABLE step_executions
    ADD COLUMN is_compensation BOOLEAN NOT NULL DEFAULT FALSE AFTER locked_until,
    ADD COLUMN executed_at TIMESTAMP NULL AFTER is_compensation,
    ADD COLUMN next_retry_at TIMESTAMP NULL AFTER executed_at;

CREATE INDEX idx_step_exec_queue ON step_executions (status, locked_until, next_retry_at);
