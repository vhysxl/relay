ALTER TABLE step_executions
    DROP COLUMN condition_evaluation,
    DROP COLUMN `condition`;

ALTER TABLE step_definitions
    DROP COLUMN `condition`;
