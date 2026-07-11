-- Persist execution-specific workflow stages. Additive and idempotent.
CREATE TABLE IF NOT EXISTS "StudioExecutionStage" (
    "executionId" TEXT NOT NULL REFERENCES "StudioExecution" ("id") ON DELETE CASCADE,
    "position" INTEGER NOT NULL CHECK ("position" >= 0 AND "position" < 1000),
    "name" TEXT NOT NULL CHECK (char_length("name") BETWEEN 1 AND 80),
    "status" TEXT NOT NULL CHECK ("status" IN ('pending', 'running', 'completed', 'failed', 'waiting')),
    "detail" TEXT NOT NULL DEFAULT '' CHECK (char_length("detail") <= 500),
    "tool" TEXT CHECK ("tool" IS NULL OR char_length("tool") <= 120),
    "metadata" JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof("metadata") = 'object'),
    "startedAt" TIMESTAMP,
    "completedAt" TIMESTAMP,
    "createdAt" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY ("executionId", "position"),
    CHECK ("completedAt" IS NULL OR "startedAt" IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS "StudioExecutionStage_execution_status_idx"
    ON "StudioExecutionStage" ("executionId", "status", "position");
CREATE INDEX IF NOT EXISTS "StudioExecutionStage_updatedAt_idx"
    ON "StudioExecutionStage" ("updatedAt" DESC);

-- Atomically create a run and its ordered stages so callers never observe an
-- execution without a timeline. SECURITY INVOKER preserves the caller's RLS.
CREATE OR REPLACE FUNCTION "createStudioExecutionWithStages"(
    "executionId" TEXT, "workflowId" TEXT, "workflowName" TEXT,
    "nodes" JSONB, "occurredAt" TIMESTAMP
) RETURNS SETOF "StudioExecution"
LANGUAGE plpgsql SECURITY INVOKER SET search_path = public AS $$
BEGIN
  IF jsonb_typeof("nodes") <> 'array' OR jsonb_array_length("nodes") < 1 OR jsonb_array_length("nodes") > 30 THEN
    RAISE EXCEPTION 'nodes must contain between 1 and 30 items';
  END IF;
  INSERT INTO "StudioExecution" ("id", "workflowId", "workflow", "status", "startedAt", "durationMs", "cost", "createdAt", "updatedAt")
  VALUES ("executionId", "workflowId", "workflowName", 'running', "occurredAt", 0, 0, "occurredAt", "occurredAt");
  INSERT INTO "StudioExecutionStage" ("executionId", "position", "name", "status", "detail", "metadata", "startedAt", "createdAt", "updatedAt")
  SELECT "executionId", node.ordinality - 1, node.name,
         CASE WHEN node.ordinality = 1 THEN 'running' ELSE 'pending' END,
         '', '{}'::jsonb, CASE WHEN node.ordinality = 1 THEN "occurredAt" ELSE NULL END,
         "occurredAt", "occurredAt"
  FROM jsonb_array_elements_text("nodes") WITH ORDINALITY AS node(name, ordinality);
  RETURN QUERY SELECT e.* FROM "StudioExecution" e WHERE e."id" = "executionId";
END;
$$;

-- Seed every existing execution from its workflow node list without replacing
-- stages that an executor has already written.
INSERT INTO "StudioExecutionStage" ("executionId", "position", "name", "status", "detail", "startedAt", "completedAt", "createdAt", "updatedAt")
SELECT e."id", node.ordinality - 1, node.name,
       CASE
         WHEN e."status" = 'completed' THEN 'completed'
         WHEN e."status" = 'failed' AND node.ordinality = 1 THEN 'failed'
         WHEN e."status" = 'waiting' AND node.ordinality = 1 THEN 'waiting'
         WHEN e."status" IN ('running', 'approved') AND node.ordinality = 1 THEN 'running'
         ELSE 'pending'
       END,
       '',
       CASE
         WHEN e."status" = 'completed' THEN e."startedAt"
         WHEN e."status" IN ('failed', 'waiting', 'running', 'approved') AND node.ordinality = 1 THEN e."startedAt"
         ELSE NULL
       END,
       CASE WHEN e."status" = 'completed' THEN e."updatedAt" ELSE NULL END,
       e."createdAt", e."updatedAt"
FROM "StudioExecution" e
JOIN "StudioWorkflow" w ON w."id" = e."workflowId"
CROSS JOIN LATERAL jsonb_array_elements_text(w."nodes") WITH ORDINALITY AS node(name, ordinality)
ON CONFLICT ("executionId", "position") DO NOTHING;
