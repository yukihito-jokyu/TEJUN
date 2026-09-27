-- +goose Up
UPDATE check_items SET human_evidence_requirement = CASE
    WHEN trim(suggested_command) <> '' THEN 'text_or_image' ELSE 'none' END;
UPDATE execution_checks SET human_evidence_requirement = CASE
    WHEN trim(suggested_command) <> '' THEN 'text_or_image' ELSE 'none' END
WHERE human_status <> 'completed' AND execution_id IN
    (SELECT execution_id FROM executions WHERE status = 'active');

-- +goose Down
SELECT 1;
