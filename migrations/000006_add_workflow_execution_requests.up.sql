CREATE TABLE workflow_execution_requests (
    workflow_name           VARCHAR(255) NOT NULL,
    idempotency_key         VARBINARY(128) NOT NULL,
    workflow_execution_id   VARCHAR(36) NOT NULL,
    input_context           LONGBLOB NOT NULL,
    response_receipt        LONGBLOB NOT NULL,
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (workflow_name, idempotency_key),
    UNIQUE KEY uq_wf_exec_req_execution (workflow_execution_id),
    CONSTRAINT fk_wf_exec_requests_execution
        FOREIGN KEY (workflow_execution_id) REFERENCES workflow_executions(id)
        ON DELETE CASCADE
);
