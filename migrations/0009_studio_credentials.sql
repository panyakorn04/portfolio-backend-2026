-- Encrypted Studio credential metadata. Secret material is encrypted by the API
-- before persistence; only the service-role backend may access this table.
CREATE TABLE IF NOT EXISTS "StudioCredential" (
    "id"            TEXT PRIMARY KEY,
    "name"          TEXT NOT NULL CHECK (char_length("name") BETWEEN 2 AND 120),
    "type"          TEXT NOT NULL CHECK ("type" IN ('bearer', 'basic', 'header', 'query')),
    "encryptedData" TEXT NOT NULL CHECK (char_length("encryptedData") BETWEEN 20 AND 20000),
    "createdAt"     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "StudioCredential_updatedAt_idx"
    ON "StudioCredential" ("updatedAt" DESC);

ALTER TABLE "StudioCredential" ENABLE ROW LEVEL SECURITY;
COMMENT ON TABLE "StudioCredential" IS 'Encrypted Studio HTTP credentials. Never return encryptedData from API list/detail responses.';

-- Keep the append-only audit allowlist aligned with all Studio mutations.
ALTER TABLE "StudioAuditLog" DROP CONSTRAINT IF EXISTS "StudioAuditLog_action_check";
ALTER TABLE "StudioAuditLog" ADD CONSTRAINT "StudioAuditLog_action_check" CHECK (
    "action" IN (
        'workflow.create', 'workflow.update', 'workflow.delete',
        'execution.create', 'execution.pause', 'execution.retry', 'execution.cancel', 'execution.approve', 'execution.run',
        'node.execute',
        'credential.create', 'credential.update', 'credential.delete', 'credential.test'
    )
);

ALTER TABLE "StudioAuditLog" DROP CONSTRAINT IF EXISTS "StudioAuditLog_resourceType_check";
ALTER TABLE "StudioAuditLog" ADD CONSTRAINT "StudioAuditLog_resourceType_check" CHECK (
    "resourceType" IN ('workflow', 'execution', 'workflow-node', 'credential')
);
