ALTER TABLE step_definitions
    ADD COLUMN `condition` JSON NULL AFTER irreversible;

ALTER TABLE step_executions
    ADD COLUMN `condition` JSON NULL AFTER is_compensation,
    ADD COLUMN condition_evaluation JSON NULL AFTER `condition`;
