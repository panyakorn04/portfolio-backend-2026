-- Persistent Studio portfolio workflows and execution history.
CREATE TABLE IF NOT EXISTS "StudioWorkflow" (
    "id"          TEXT PRIMARY KEY,
    "name"        TEXT NOT NULL CHECK (char_length("name") BETWEEN 2 AND 120),
    "description" TEXT NOT NULL DEFAULT '' CHECK (char_length("description") <= 1000),
    "category"    TEXT NOT NULL CHECK (char_length("category") BETWEEN 2 AND 80),
    "status"      TEXT NOT NULL CHECK ("status" IN ('active', 'draft', 'paused')),
    "runs"        INTEGER NOT NULL DEFAULT 0 CHECK ("runs" >= 0),
    "success"     DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK ("success" BETWEEN 0 AND 100),
    "nodes"       JSONB NOT NULL DEFAULT '[]'::jsonb,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "StudioWorkflow_status_updatedAt_idx"
    ON "StudioWorkflow" ("status", "updatedAt" DESC);

CREATE TABLE IF NOT EXISTS "StudioExecution" (
    "id"          TEXT PRIMARY KEY,
    "workflowId"  TEXT NOT NULL REFERENCES "StudioWorkflow" ("id") ON DELETE RESTRICT,
    "workflow"    TEXT NOT NULL CHECK (char_length("workflow") BETWEEN 2 AND 120),
    "status"      TEXT NOT NULL CHECK ("status" IN ('running', 'paused', 'waiting', 'approved', 'completed', 'failed', 'cancelled')),
    "startedAt"   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "durationMs"  INTEGER NOT NULL DEFAULT 0 CHECK ("durationMs" >= 0),
    "cost"        DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK ("cost" >= 0),
    "createdAt"   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "StudioExecution_startedAt_idx" ON "StudioExecution" ("startedAt" DESC);
CREATE INDEX IF NOT EXISTS "StudioExecution_workflowId_startedAt_idx" ON "StudioExecution" ("workflowId", "startedAt" DESC);
CREATE INDEX IF NOT EXISTS "StudioExecution_status_idx" ON "StudioExecution" ("status");
