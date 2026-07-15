-- Atomically lock execution ownership before completing a stage, and retire the
-- legacy browser-callable execution creation RPC.

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
BEGIN
    IF "stageStatus" NOT IN ('completed', 'failed', 'cancelled') THEN
        RAISE EXCEPTION 'stageStatus is invalid';
    END IF;

    SELECT execution."id" INTO locked_execution_id
      FROM "StudioExecution" execution
     WHERE execution."id" = "executionId"
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
     WHERE stage."executionId" = "executionId" AND stage."position" = "stagePosition"
       AND stage."status" = 'running';
    GET DIAGNOSTICS changed = ROW_COUNT;
    RETURN changed = 1;
END;
$$;

REVOKE EXECUTE ON FUNCTION "finishStudioExecutionStage"(TEXT,INTEGER,TEXT,TEXT,JSONB,TEXT,TEXT,TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION "finishStudioExecutionStage"(TEXT,INTEGER,TEXT,TEXT,JSONB,TEXT,TEXT,TEXT,TIMESTAMP) TO service_role;

REVOKE EXECUTE ON FUNCTION "createStudioExecutionWithStages"(TEXT,TEXT,TEXT,JSONB,TIMESTAMP) FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION "createStudioExecutionWithStages"(TEXT,TEXT,TEXT,JSONB,TIMESTAMP) TO service_role;

NOTIFY pgrst, 'reload schema';
