-- Immutable audit trail for authenticated Studio mutations.
CREATE TABLE IF NOT EXISTS "StudioAuditLog" (
    "id"           TEXT PRIMARY KEY,
    "actorType"    TEXT NOT NULL CHECK ("actorType" IN ('session', 'bearer')),
    "actorId"      TEXT NULL,
    "actorLabel"   TEXT NOT NULL CHECK (char_length("actorLabel") BETWEEN 1 AND 254),
    "action"       TEXT NOT NULL CHECK ("action" IN ('workflow.create', 'workflow.update', 'execution.pause', 'execution.retry', 'execution.cancel', 'execution.approve')),
    "resourceType" TEXT NOT NULL CHECK ("resourceType" IN ('workflow', 'execution')),
    "resourceId"   TEXT NOT NULL CHECK (char_length("resourceId") BETWEEN 1 AND 128),
    "fromStatus"   TEXT NULL,
    "toStatus"     TEXT NULL,
    "metadata"     JSONB NOT NULL DEFAULT '{}'::jsonb,
    "createdAt"    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "StudioAuditLog_createdAt_idx" ON "StudioAuditLog" ("createdAt" DESC);
CREATE INDEX IF NOT EXISTS "StudioAuditLog_resource_idx" ON "StudioAuditLog" ("resourceType", "resourceId", "createdAt" DESC);
CREATE INDEX IF NOT EXISTS "StudioAuditLog_actor_idx" ON "StudioAuditLog" ("actorId", "createdAt" DESC) WHERE "actorId" IS NOT NULL;

-- Audit rows are append-only for API roles; the service only uses GET/POST.
COMMENT ON TABLE "StudioAuditLog" IS 'Append-only Studio admin mutation audit trail; do not store credentials or bearer/session tokens.';
