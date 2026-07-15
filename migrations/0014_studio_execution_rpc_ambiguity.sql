-- Eliminate PL/pgSQL parameter/column ambiguity from execution lifecycle RPCs.

CREATE OR REPLACE FUNCTION "startStudioExecutionStage"(
    "executionId" TEXT,
    "stagePosition" INTEGER,
    "workerId" TEXT,
    "stageInput" JSONB,
    "leaseSeconds" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE
    changed INTEGER;
    p_execution_id TEXT := "executionId";
BEGIN
    UPDATE "StudioExecution" execution
       SET "leaseUntil" = "occurredAt" + make_interval(secs => "leaseSeconds"), "updatedAt" = "occurredAt"
     WHERE execution."id" = p_execution_id AND execution."leaseOwner" = "workerId"
       AND execution."status" = 'running' AND execution."leaseUntil" >= LOCALTIMESTAMP;
    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;

    UPDATE "StudioExecutionStage" stage
       SET "status" = 'running', "input" = "stageInput", "startedAt" = COALESCE(stage."startedAt", "occurredAt"),
           "updatedAt" = "occurredAt"
     WHERE stage."executionId" = p_execution_id AND stage."position" = "stagePosition"
       AND stage."status" IN ('pending', 'running');
    GET DIAGNOSTICS changed = ROW_COUNT;
    RETURN changed = 1;
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
DECLARE
    changed INTEGER;
    locked_execution_id TEXT;
    p_execution_id TEXT := "executionId";
BEGIN
    IF "stageStatus" NOT IN ('completed', 'failed', 'cancelled') THEN
        RAISE EXCEPTION 'stageStatus is invalid';
    END IF;

    SELECT execution."id" INTO locked_execution_id
      FROM "StudioExecution" execution
     WHERE execution."id" = p_execution_id
       AND execution."leaseOwner" = "workerId"
       AND execution."status" IN ('running', 'cancellation_requested')
       AND execution."leaseUntil" >= LOCALTIMESTAMP
     FOR UPDATE;
    IF locked_execution_id IS NULL THEN
        RETURN FALSE;
    END IF;

    UPDATE "StudioExecutionStage" stage
       SET "status" = "stageStatus", "output" = "stageOutput", "errorCode" = "stageErrorCode",
           "errorMessage" = "stageErrorMessage", "detail" = LEFT(COALESCE("stageDetail", ''), 500),
           "completedAt" = "occurredAt",
           "durationMs" = GREATEST(0, FLOOR(EXTRACT(EPOCH FROM ("occurredAt" - COALESCE(stage."startedAt", "occurredAt"))) * 1000)::INTEGER),
           "updatedAt" = "occurredAt"
     WHERE stage."executionId" = p_execution_id AND stage."position" = "stagePosition"
       AND stage."status" = 'running';
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
DECLARE
    p_execution_id TEXT := "executionId";
    final_status TEXT;
BEGIN
    IF "executionStatus" NOT IN ('completed', 'failed', 'cancelled') THEN
        RAISE EXCEPTION 'executionStatus is invalid';
    END IF;

    UPDATE "StudioExecution" execution
       SET "status" = CASE WHEN execution."status" = 'cancellation_requested' THEN 'cancelled' ELSE "executionStatus" END,
           "errorCode" = CASE WHEN execution."status" = 'cancellation_requested' THEN 'cancelled' ELSE "executionErrorCode" END,
           "errorMessage" = CASE WHEN execution."status" = 'cancellation_requested' THEN 'Execution cancelled.' ELSE "executionErrorMessage" END,
           "completedAt" = "occurredAt",
           "durationMs" = GREATEST(0, FLOOR(EXTRACT(EPOCH FROM ("occurredAt" - execution."startedAt")) * 1000)::INTEGER),
           "leaseOwner" = NULL, "leaseUntil" = NULL, "updatedAt" = "occurredAt"
     WHERE execution."id" = p_execution_id AND execution."leaseOwner" = "workerId"
       AND execution."status" IN ('running', 'cancellation_requested')
       AND execution."leaseUntil" >= LOCALTIMESTAMP
    RETURNING execution."status" INTO final_status;
    IF final_status IS NULL THEN
        RETURN FALSE;
    END IF;

    UPDATE "StudioExecutionStage" stage
       SET "status" = CASE WHEN final_status = 'cancelled' THEN 'cancelled' ELSE 'skipped' END,
           "detail" = CASE WHEN final_status = 'cancelled' THEN 'Execution cancelled' ELSE 'Skipped after execution stopped' END,
           "completedAt" = "occurredAt", "updatedAt" = "occurredAt"
     WHERE stage."executionId" = p_execution_id AND stage."status" = 'pending';
    RETURN TRUE;
END;
$$;

CREATE OR REPLACE FUNCTION "cancelStudioGraphExecution"(
    "executionId" TEXT,
    "occurredAt" TIMESTAMP
) RETURNS SETOF "StudioExecution"
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE
    p_execution_id TEXT := "executionId";
    final_status TEXT;
BEGIN
    UPDATE "StudioExecution" execution
       SET "status" = CASE WHEN execution."status" = 'queued' THEN 'cancelled' ELSE 'cancellation_requested' END,
           "cancellationRequestedAt" = "occurredAt",
           "completedAt" = CASE WHEN execution."status" = 'queued' THEN "occurredAt" ELSE execution."completedAt" END,
           "updatedAt" = "occurredAt"
     WHERE execution."id" = p_execution_id AND execution."status" IN ('queued', 'running')
    RETURNING execution."status" INTO final_status;

    IF final_status IS NULL THEN
        RETURN;
    END IF;
    IF final_status = 'cancelled' THEN
        UPDATE "StudioExecutionStage" stage
           SET "status" = 'cancelled', "detail" = 'Execution cancelled', "completedAt" = "occurredAt", "updatedAt" = "occurredAt"
         WHERE stage."executionId" = p_execution_id AND stage."status" = 'pending';
    END IF;
    RETURN QUERY SELECT execution.* FROM "StudioExecution" execution WHERE execution."id" = p_execution_id;
END;
$$;

CREATE OR REPLACE FUNCTION "deleteStudioWorkflowIfIdle"(
    "workflowId" TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE
    locked_id TEXT;
    p_workflow_id TEXT := "workflowId";
BEGIN
    SELECT workflow."id" INTO locked_id
      FROM "StudioWorkflow" workflow
     WHERE workflow."id" = p_workflow_id
     FOR UPDATE;
    IF locked_id IS NULL THEN
        RETURN FALSE;
    END IF;
    IF EXISTS (
        SELECT 1 FROM "StudioExecution" execution
         WHERE execution."workflowId" = p_workflow_id
           AND execution."status" IN ('queued', 'running', 'cancellation_requested')
    ) THEN
        RETURN FALSE;
    END IF;
    DELETE FROM "StudioWorkflow" workflow WHERE workflow."id" = p_workflow_id;
    RETURN FOUND;
END;
$$;

REVOKE EXECUTE ON FUNCTION "startStudioExecutionStage"(TEXT,INTEGER,TEXT,JSONB,INTEGER,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "finishStudioExecutionStage"(TEXT,INTEGER,TEXT,TEXT,JSONB,TEXT,TEXT,TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "finishStudioGraphExecution"(TEXT,TEXT,TEXT,TEXT,TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "cancelStudioGraphExecution"(TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "deleteStudioWorkflowIfIdle"(TEXT) FROM PUBLIC, anon, authenticated;

GRANT EXECUTE ON FUNCTION "startStudioExecutionStage"(TEXT,INTEGER,TEXT,JSONB,INTEGER,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "finishStudioExecutionStage"(TEXT,INTEGER,TEXT,TEXT,JSONB,TEXT,TEXT,TEXT,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "finishStudioGraphExecution"(TEXT,TEXT,TEXT,TEXT,TEXT,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "cancelStudioGraphExecution"(TEXT,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "deleteStudioWorkflowIfIdle"(TEXT) TO service_role;

NOTIFY pgrst, 'reload schema';
