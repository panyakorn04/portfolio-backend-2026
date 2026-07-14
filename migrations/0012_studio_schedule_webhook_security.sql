-- Fix execution idempotency fallback ambiguity for schedule/webhook replays.

CREATE OR REPLACE FUNCTION "enqueueStudioGraphExecution"(
    "executionId" TEXT,
    "workflowId" TEXT,
    "workflowName" TEXT,
    "workflowUpdatedAt" TIMESTAMP,
    "triggerNodeId" TEXT,
    "targetNodeId" TEXT,
    "executionMode" TEXT,
    "executionSource" TEXT,
    "sourceKey" TEXT,
    "retryOfExecutionId" TEXT,
    "initialInput" JSONB,
    "pathNodes" JSONB,
    "occurredAt" TIMESTAMP
) RETURNS SETOF "StudioExecution"
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
<<enqueue_block>>
DECLARE
    p_source_key TEXT := "sourceKey";
BEGIN
    IF jsonb_typeof(COALESCE("initialInput", '[]'::jsonb)) <> 'array' OR octet_length(COALESCE("initialInput", '[]'::jsonb)::text) > 262144 THEN
        RAISE EXCEPTION 'initialInput is invalid';
    END IF;
    IF jsonb_typeof("pathNodes") <> 'array' OR jsonb_array_length("pathNodes") < 1 OR jsonb_array_length("pathNodes") > 30 THEN
        RAISE EXCEPTION 'pathNodes must contain between 1 and 30 items';
    END IF;
    IF EXISTS (
        SELECT 1 FROM jsonb_array_elements("pathNodes") node
        WHERE jsonb_typeof(node) <> 'object'
           OR COALESCE(node->>'id', '') = ''
           OR COALESCE(node->>'type', '') = ''
           OR COALESCE(node->>'label', '') = ''
    ) THEN
        RAISE EXCEPTION 'pathNodes are invalid';
    END IF;

    INSERT INTO "StudioExecution" (
        "id", "workflowId", "workflow", "status", "startedAt", "durationMs", "cost",
        "triggerNodeId", "targetNodeId", "mode", "source", "sourceKey", "workflowUpdatedAt",
        "retryOfExecutionId", "createdAt", "updatedAt"
    ) VALUES (
        "executionId", "workflowId", "workflowName", 'queued', "occurredAt", 0, 0,
        "triggerNodeId", "targetNodeId", "executionMode", "executionSource", enqueue_block.p_source_key, "workflowUpdatedAt",
        "retryOfExecutionId", "occurredAt", "occurredAt"
    );

    INSERT INTO "StudioExecutionStage" (
        "executionId", "position", "nodeId", "nodeType", "name", "status", "detail", "metadata", "input",
        "createdAt", "updatedAt"
    )
    SELECT "executionId", node.ordinality - 1, node.value->>'id', node.value->>'type', node.value->>'label',
           'pending', '', '{}'::jsonb, CASE WHEN node.ordinality = 1 THEN COALESCE("initialInput", '[]'::jsonb) ELSE NULL END,
           "occurredAt", "occurredAt"
    FROM jsonb_array_elements("pathNodes") WITH ORDINALITY AS node(value, ordinality);

    RETURN QUERY SELECT execution.* FROM "StudioExecution" execution WHERE execution."id" = "executionId";
EXCEPTION
    WHEN unique_violation THEN
        IF enqueue_block.p_source_key IS NOT NULL THEN
            RETURN QUERY
            SELECT execution.*
              FROM "StudioExecution" execution
             WHERE execution."sourceKey" = enqueue_block.p_source_key;
        ELSE
            RAISE;
        END IF;
END enqueue_block;
$$;

REVOKE EXECUTE ON FUNCTION "enqueueStudioGraphExecution"(TEXT,TEXT,TEXT,TIMESTAMP,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TIMESTAMP) FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION "enqueueStudioGraphExecution"(TEXT,TEXT,TEXT,TIMESTAMP,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TIMESTAMP) TO service_role;

NOTIFY pgrst, 'reload schema';
