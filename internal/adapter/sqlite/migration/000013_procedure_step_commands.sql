-- +goose Up
UPDATE procedures AS p
SET document_json = (
    WITH RECURSIVE rewritten(position, document) AS (
        SELECT 0, p.document_json
        UNION ALL
        SELECT position + 1,
            CASE WHEN json_type(document, '$.steps[' || position || '].command') IS NULL
                THEN json_set(document, '$.steps[' || position || '].command', COALESCE((
                    SELECT ec.suggested_command FROM execution_checks ec
                    JOIN procedure_sources ps ON ps.execution_id = ec.execution_id
                    WHERE ps.procedure_id = p.procedure_id
                      AND ec.check_id = COALESCE(
                          json_extract(document, '$.steps[' || position || '].stepId'),
                          json_extract(document, '$.steps[' || position || '].clientKey'))
                ), ''))
                ELSE document END
        FROM rewritten
        WHERE position < json_array_length(document, '$.steps')
    )
    SELECT document FROM rewritten ORDER BY position DESC LIMIT 1
)
WHERE p.revision = 1 AND p.status = 'draft';

-- +goose Down
SELECT 1;
