-- Phase 3: database-backed Studio graph execution queue and persisted node I/O.
-- Additive/idempotent so it can be applied after existing Studio migrations.

ALTER TABLE "StudioExecution" DROP CONSTRAINT IF EXISTS "StudioExecution_workflowId_fkey";
ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_workflowId_fkey"
    FOREIGN KEY ("workflowId") REFERENCES "StudioWorkflow" ("id") ON DELETE CASCADE;

ALTER TABLE "StudioExecution"
    ADD COLUMN IF NOT EXISTS "triggerNodeId" TEXT,
    ADD COLUMN IF NOT EXISTS "targetNodeId" TEXT,
    ADD COLUMN IF NOT EXISTS "mode" TEXT NOT NULL DEFAULT 'full',
    ADD COLUMN IF NOT EXISTS "source" TEXT NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS "sourceKey" TEXT,
    ADD COLUMN IF NOT EXISTS "workflowUpdatedAt" TIMESTAMP,
    ADD COLUMN IF NOT EXISTS "completedAt" TIMESTAMP,
    ADD COLUMN IF NOT EXISTS "errorCode" TEXT,
    ADD COLUMN IF NOT EXISTS "errorMessage" TEXT,
    ADD COLUMN IF NOT EXISTS "cancellationRequestedAt" TIMESTAMP,
    ADD COLUMN IF NOT EXISTS "leaseOwner" TEXT,
    ADD COLUMN IF NOT EXISTS "leaseUntil" TIMESTAMP,
    ADD COLUMN IF NOT EXISTS "retryOfExecutionId" TEXT;

ALTER TABLE "StudioExecution" DROP CONSTRAINT IF EXISTS "StudioExecution_status_check";
ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_status_check" CHECK (
    "status" IN ('queued', 'running', 'cancellation_requested', 'paused', 'waiting', 'approved', 'completed', 'failed', 'cancelled')
);
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'StudioExecution_retryOfExecutionId_fkey') THEN
        ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_retryOfExecutionId_fkey"
            FOREIGN KEY ("retryOfExecutionId") REFERENCES "StudioExecution" ("id") ON DELETE SET NULL;
    END IF;
END $$;
ALTER TABLE "StudioExecution" DROP CONSTRAINT IF EXISTS "StudioExecution_mode_check";
ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_mode_check"
    CHECK ("mode" IN ('full', 'through-target'));
ALTER TABLE "StudioExecution" DROP CONSTRAINT IF EXISTS "StudioExecution_source_check";
ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_source_check"
    CHECK ("source" IN ('manual', 'schedule', 'webhook', 'node-test', 'retry'));
ALTER TABLE "StudioExecution" DROP CONSTRAINT IF EXISTS "StudioExecution_errorCode_length_check";
ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_errorCode_length_check"
    CHECK ("errorCode" IS NULL OR char_length("errorCode") <= 80);
ALTER TABLE "StudioExecution" DROP CONSTRAINT IF EXISTS "StudioExecution_errorMessage_length_check";
ALTER TABLE "StudioExecution" ADD CONSTRAINT "StudioExecution_errorMessage_length_check"
    CHECK ("errorMessage" IS NULL OR char_length("errorMessage") <= 500);
CREATE UNIQUE INDEX IF NOT EXISTS "StudioExecution_sourceKey_unique_idx"
    ON "StudioExecution" ("sourceKey") WHERE "sourceKey" IS NOT NULL;
CREATE INDEX IF NOT EXISTS "StudioExecution_queue_idx"
    ON "StudioExecution" ("status", "leaseUntil", "createdAt");

ALTER TABLE "StudioExecutionStage"
    ADD COLUMN IF NOT EXISTS "nodeId" TEXT,
    ADD COLUMN IF NOT EXISTS "nodeType" TEXT,
    ADD COLUMN IF NOT EXISTS "input" JSONB,
    ADD COLUMN IF NOT EXISTS "output" JSONB,
    ADD COLUMN IF NOT EXISTS "errorCode" TEXT,
    ADD COLUMN IF NOT EXISTS "errorMessage" TEXT,
    ADD COLUMN IF NOT EXISTS "durationMs" INTEGER NOT NULL DEFAULT 0;
ALTER TABLE "StudioExecutionStage" DROP CONSTRAINT IF EXISTS "StudioExecutionStage_status_check";
ALTER TABLE "StudioExecutionStage" ADD CONSTRAINT "StudioExecutionStage_status_check" CHECK (
    "status" IN ('pending', 'running', 'completed', 'failed', 'waiting', 'skipped', 'cancelled')
);
ALTER TABLE "StudioExecutionStage" DROP CONSTRAINT IF EXISTS "StudioExecutionStage_input_shape_check";
ALTER TABLE "StudioExecutionStage" ADD CONSTRAINT "StudioExecutionStage_input_shape_check" CHECK (
    "input" IS NULL OR (jsonb_typeof("input") = 'array' AND octet_length("input"::text) <= 262144)
);
ALTER TABLE "StudioExecutionStage" DROP CONSTRAINT IF EXISTS "StudioExecutionStage_output_shape_check";
ALTER TABLE "StudioExecutionStage" ADD CONSTRAINT "StudioExecutionStage_output_shape_check" CHECK (
    "output" IS NULL OR (jsonb_typeof("output") = 'array' AND octet_length("output"::text) <= 262144)
);
ALTER TABLE "StudioExecutionStage" DROP CONSTRAINT IF EXISTS "StudioExecutionStage_errorCode_length_check";
ALTER TABLE "StudioExecutionStage" ADD CONSTRAINT "StudioExecutionStage_errorCode_length_check"
    CHECK ("errorCode" IS NULL OR char_length("errorCode") <= 80);
ALTER TABLE "StudioExecutionStage" DROP CONSTRAINT IF EXISTS "StudioExecutionStage_errorMessage_length_check";
ALTER TABLE "StudioExecutionStage" ADD CONSTRAINT "StudioExecutionStage_errorMessage_length_check"
    CHECK ("errorMessage" IS NULL OR char_length("errorMessage") <= 500);
ALTER TABLE "StudioExecutionStage" DROP CONSTRAINT IF EXISTS "StudioExecutionStage_durationMs_check";
ALTER TABLE "StudioExecutionStage" ADD CONSTRAINT "StudioExecutionStage_durationMs_check" CHECK ("durationMs" >= 0);

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
        "triggerNodeId", "targetNodeId", "executionMode", "executionSource", "sourceKey", "workflowUpdatedAt",
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
        IF "sourceKey" IS NOT NULL THEN
            RETURN QUERY SELECT execution.* FROM "StudioExecution" execution WHERE execution."sourceKey" = "sourceKey";
        ELSE
            RAISE;
        END IF;
END;
$$;

CREATE OR REPLACE FUNCTION "claimStudioGraphExecution"(
    "workerId" TEXT,
    "leaseSeconds" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS SETOF "StudioExecution"
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE
    claimed_id TEXT;
BEGIN
    IF "leaseSeconds" < 30 OR "leaseSeconds" > 300 THEN
        RAISE EXCEPTION 'leaseSeconds is invalid';
    END IF;
    SELECT execution."id" INTO claimed_id
    FROM "StudioExecution" execution
    WHERE execution."status" = 'queued'
       OR (execution."status" = 'running' AND execution."leaseUntil" < "occurredAt")
    ORDER BY execution."createdAt" ASC
    FOR UPDATE SKIP LOCKED
    LIMIT 1;

    IF claimed_id IS NULL THEN
        RETURN;
    END IF;

    RETURN QUERY
    UPDATE "StudioExecution" execution
       SET "status" = 'running', "leaseOwner" = "workerId",
           "leaseUntil" = "occurredAt" + make_interval(secs => "leaseSeconds"),
           "updatedAt" = "occurredAt"
     WHERE execution."id" = claimed_id
    RETURNING execution.*;
END;
$$;

CREATE OR REPLACE FUNCTION "startStudioExecutionStage"(
    "executionId" TEXT,
    "stagePosition" INTEGER,
    "workerId" TEXT,
    "stageInput" JSONB,
    "leaseSeconds" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE changed INTEGER;
BEGIN
    UPDATE "StudioExecutionStage" stage
       SET "status" = 'running', "input" = "stageInput", "startedAt" = COALESCE(stage."startedAt", "occurredAt"),
           "updatedAt" = "occurredAt"
     WHERE stage."executionId" = "executionId" AND stage."position" = "stagePosition"
       AND stage."status" IN ('pending', 'running');
    GET DIAGNOSTICS changed = ROW_COUNT;
    UPDATE "StudioExecution" execution
       SET "leaseUntil" = "occurredAt" + make_interval(secs => "leaseSeconds"), "updatedAt" = "occurredAt"
     WHERE execution."id" = "executionId" AND execution."leaseOwner" = "workerId"
       AND execution."status" = 'running';
    RETURN changed = 1 AND FOUND;
END;
$$;

CREATE OR REPLACE FUNCTION "finishStudioExecutionStage"(
    "executionId" TEXT,
    "stagePosition" INTEGER,
    "workerId" TEXT,
    "stageStatus" TEXT,
    "stageOutput" JSONB,
    "stageErrorCode" TEXT,
    "stageErrorMessage" TEXT,
    "stageDetail" TEXT,
    "occurredAt" TIMESTAMP
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE changed INTEGER;
BEGIN
    IF "stageStatus" NOT IN ('completed', 'failed', 'cancelled') THEN
        RAISE EXCEPTION 'stageStatus is invalid';
    END IF;
    UPDATE "StudioExecutionStage" stage
       SET "status" = "stageStatus", "output" = "stageOutput", "errorCode" = "stageErrorCode",
           "errorMessage" = "stageErrorMessage", "detail" = LEFT(COALESCE("stageDetail", ''), 500),
           "completedAt" = "occurredAt",
           "durationMs" = GREATEST(0, FLOOR(EXTRACT(EPOCH FROM ("occurredAt" - COALESCE(stage."startedAt", "occurredAt"))) * 1000)::INTEGER),
           "updatedAt" = "occurredAt"
     WHERE stage."executionId" = "executionId" AND stage."position" = "stagePosition"
       AND stage."status" = 'running'
       AND EXISTS (SELECT 1 FROM "StudioExecution" execution WHERE execution."id" = "executionId" AND execution."leaseOwner" = "workerId");
    GET DIAGNOSTICS changed = ROW_COUNT;
    RETURN changed = 1;
END;
$$;

CREATE OR REPLACE FUNCTION "finishStudioGraphExecution"(
    "executionId" TEXT,
    "workerId" TEXT,
    "executionStatus" TEXT,
    "executionErrorCode" TEXT,
    "executionErrorMessage" TEXT,
    "occurredAt" TIMESTAMP
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE changed INTEGER;
BEGIN
    IF "executionStatus" NOT IN ('completed', 'failed', 'cancelled') THEN
        RAISE EXCEPTION 'executionStatus is invalid';
    END IF;
    UPDATE "StudioExecution" execution
       SET "status" = CASE WHEN execution."status" = 'cancellation_requested' THEN 'cancelled' ELSE "executionStatus" END,
           "errorCode" = CASE WHEN execution."status" = 'cancellation_requested' THEN 'cancelled' ELSE "executionErrorCode" END,
           "errorMessage" = CASE WHEN execution."status" = 'cancellation_requested' THEN 'Execution cancelled.' ELSE "executionErrorMessage" END,
           "completedAt" = "occurredAt", "durationMs" = GREATEST(0, FLOOR(EXTRACT(EPOCH FROM ("occurredAt" - execution."startedAt")) * 1000)::INTEGER),
           "leaseOwner" = NULL, "leaseUntil" = NULL, "updatedAt" = "occurredAt"
     WHERE execution."id" = "executionId" AND execution."leaseOwner" = "workerId"
       AND execution."status" IN ('running', 'cancellation_requested');
    GET DIAGNOSTICS changed = ROW_COUNT;
    IF changed = 1 THEN
        UPDATE "StudioExecutionStage" stage
           SET "status" = CASE WHEN (SELECT execution."status" FROM "StudioExecution" execution WHERE execution."id" = "executionId") = 'cancelled' THEN 'cancelled' ELSE 'skipped' END,
               "detail" = CASE WHEN (SELECT execution."status" FROM "StudioExecution" execution WHERE execution."id" = "executionId") = 'cancelled' THEN 'Execution cancelled' ELSE 'Skipped after execution stopped' END,
               "completedAt" = "occurredAt", "updatedAt" = "occurredAt"
         WHERE stage."executionId" = "executionId" AND stage."status" = 'pending';
    END IF;
    RETURN changed = 1;
END;
$$;

CREATE OR REPLACE FUNCTION "cancelStudioGraphExecution"(
    "executionId" TEXT,
    "occurredAt" TIMESTAMP
) RETURNS SETOF "StudioExecution"
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
BEGIN
    UPDATE "StudioExecution" execution
       SET "status" = CASE WHEN execution."status" = 'queued' THEN 'cancelled' ELSE 'cancellation_requested' END,
           "cancellationRequestedAt" = "occurredAt",
           "completedAt" = CASE WHEN execution."status" = 'queued' THEN "occurredAt" ELSE execution."completedAt" END,
           "updatedAt" = "occurredAt"
     WHERE execution."id" = "executionId" AND execution."status" IN ('queued', 'running');
    IF FOUND THEN
        UPDATE "StudioExecutionStage" stage
           SET "status" = 'cancelled', "detail" = 'Execution cancelled', "completedAt" = "occurredAt", "updatedAt" = "occurredAt"
         WHERE stage."executionId" = "executionId" AND stage."status" = 'pending'
           AND EXISTS (SELECT 1 FROM "StudioExecution" execution WHERE execution."id" = "executionId" AND execution."status" = 'cancelled');
    END IF;
    RETURN QUERY SELECT execution.* FROM "StudioExecution" execution WHERE execution."id" = "executionId";
END;
$$;

CREATE OR REPLACE FUNCTION "deleteStudioWorkflowIfIdle"(
    "workflowId" TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE locked_id TEXT;
BEGIN
    SELECT workflow."id" INTO locked_id
      FROM "StudioWorkflow" workflow
     WHERE workflow."id" = "workflowId"
     FOR UPDATE;
    IF locked_id IS NULL THEN
        RETURN FALSE;
    END IF;
    IF EXISTS (
        SELECT 1 FROM "StudioExecution" execution
         WHERE execution."workflowId" = "workflowId"
           AND execution."status" IN ('queued', 'running', 'cancellation_requested')
    ) THEN
        RETURN FALSE;
    END IF;
    DELETE FROM "StudioWorkflow" workflow WHERE workflow."id" = "workflowId";
    RETURN FOUND;
END;
$$;
