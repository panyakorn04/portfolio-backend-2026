-- Lock down Studio execution internals and harden lease/crash recovery.

CREATE OR REPLACE FUNCTION "claimStudioGraphExecution"(
    "workerId" TEXT,
    "leaseSeconds" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS SETOF "StudioExecution"
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
DECLARE
    claimed_id TEXT;
    recovered_id TEXT;
BEGIN
    IF "leaseSeconds" < 30 OR "leaseSeconds" > 300 THEN
        RAISE EXCEPTION 'leaseSeconds is invalid';
    END IF;

    FOR recovered_id IN
        SELECT execution."id"
          FROM "StudioExecution" execution
         WHERE execution."status" = 'cancellation_requested'
           AND execution."leaseUntil" < "occurredAt"
         ORDER BY execution."createdAt" ASC
         FOR UPDATE SKIP LOCKED
         LIMIT 100
    LOOP
        UPDATE "StudioExecution" execution
           SET "status" = 'cancelled', "errorCode" = 'cancelled', "errorMessage" = 'Execution cancelled.',
               "completedAt" = "occurredAt", "leaseOwner" = NULL, "leaseUntil" = NULL, "updatedAt" = "occurredAt"
         WHERE execution."id" = recovered_id;
        UPDATE "StudioExecutionStage" stage
           SET "status" = 'cancelled', "detail" = 'Execution cancelled after worker recovery',
               "errorCode" = 'cancelled', "errorMessage" = 'Execution cancelled.',
               "completedAt" = "occurredAt", "updatedAt" = "occurredAt"
         WHERE stage."executionId" = recovered_id AND stage."status" IN ('pending', 'running');
    END LOOP;

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
           "errorCode" = CASE WHEN execution."status" = 'running' THEN 'lease_recovered' ELSE NULL END,
           "errorMessage" = CASE WHEN execution."status" = 'running' THEN 'Execution recovered after worker lease expiry.' ELSE NULL END,
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
    UPDATE "StudioExecution" execution
       SET "leaseUntil" = "occurredAt" + make_interval(secs => "leaseSeconds"), "updatedAt" = "occurredAt"
     WHERE execution."id" = "executionId" AND execution."leaseOwner" = "workerId"
       AND execution."status" = 'running' AND execution."leaseUntil" >= "occurredAt";
    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;
    UPDATE "StudioExecutionStage" stage
       SET "status" = 'running', "input" = "stageInput", "startedAt" = COALESCE(stage."startedAt", "occurredAt"),
           "updatedAt" = "occurredAt"
     WHERE stage."executionId" = "executionId" AND stage."position" = "stagePosition"
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
       AND EXISTS (
           SELECT 1 FROM "StudioExecution" execution
            WHERE execution."id" = "executionId" AND execution."leaseOwner" = "workerId"
              AND execution."status" IN ('running', 'cancellation_requested')
              AND execution."leaseUntil" >= "occurredAt"
       );
    GET DIAGNOSTICS changed = ROW_COUNT;
    RETURN changed = 1;
END;
$$;

REVOKE ALL ON TABLE "StudioWorkflow", "StudioExecution", "StudioExecutionStage", "StudioCredential", "StudioAuditLog" FROM PUBLIC, anon, authenticated;
GRANT ALL ON TABLE "StudioWorkflow", "StudioExecution", "StudioExecutionStage", "StudioCredential", "StudioAuditLog" TO service_role;

REVOKE EXECUTE ON FUNCTION "enqueueStudioGraphExecution"(TEXT,TEXT,TEXT,TIMESTAMP,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "claimStudioGraphExecution"(TEXT,INTEGER,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "startStudioExecutionStage"(TEXT,INTEGER,TEXT,JSONB,INTEGER,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "finishStudioExecutionStage"(TEXT,INTEGER,TEXT,TEXT,JSONB,TEXT,TEXT,TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "finishStudioGraphExecution"(TEXT,TEXT,TEXT,TEXT,TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "cancelStudioGraphExecution"(TEXT,TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE EXECUTE ON FUNCTION "deleteStudioWorkflowIfIdle"(TEXT) FROM PUBLIC, anon, authenticated;

GRANT EXECUTE ON FUNCTION "enqueueStudioGraphExecution"(TEXT,TEXT,TEXT,TIMESTAMP,TEXT,TEXT,TEXT,TEXT,TEXT,TEXT,JSONB,JSONB,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "claimStudioGraphExecution"(TEXT,INTEGER,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "startStudioExecutionStage"(TEXT,INTEGER,TEXT,JSONB,INTEGER,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "finishStudioExecutionStage"(TEXT,INTEGER,TEXT,TEXT,JSONB,TEXT,TEXT,TEXT,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "finishStudioGraphExecution"(TEXT,TEXT,TEXT,TEXT,TEXT,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "cancelStudioGraphExecution"(TEXT,TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "deleteStudioWorkflowIfIdle"(TEXT) TO service_role;
