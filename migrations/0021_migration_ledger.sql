-- Append-only migration ledger for production-state verification.
-- Historical rows are registered by a generated, operator-reviewed bootstrap bundle.

CREATE TABLE IF NOT EXISTS "PortfolioMigration" (
    "version" TEXT PRIMARY KEY CHECK ("version" ~ '^[0-9]{4}$'),
    "filename" TEXT NOT NULL UNIQUE,
    "checksum" TEXT NOT NULL CHECK ("checksum" ~ '^[0-9a-f]{64}$'),
    "appliedAt" TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "appliedBy" TEXT NOT NULL DEFAULT CURRENT_USER,
    CHECK ("filename" LIKE "version" || '\_%' ESCAPE '\')
);

ALTER TABLE "PortfolioMigration" ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION "preventPortfolioMigrationMutation"()
RETURNS TRIGGER
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
BEGIN
    RAISE EXCEPTION 'PortfolioMigration rows are immutable';
END;
$$;

DROP TRIGGER IF EXISTS "PortfolioMigration_immutable_trigger" ON "PortfolioMigration";
CREATE TRIGGER "PortfolioMigration_immutable_trigger"
BEFORE UPDATE OR DELETE ON "PortfolioMigration"
FOR EACH ROW
EXECUTE FUNCTION "preventPortfolioMigrationMutation"();

REVOKE ALL ON TABLE "PortfolioMigration" FROM PUBLIC, anon, authenticated, service_role;
GRANT SELECT ON TABLE "PortfolioMigration" TO service_role;

REVOKE ALL ON FUNCTION "preventPortfolioMigrationMutation"() FROM PUBLIC, anon, authenticated, service_role;

NOTIFY pgrst, 'reload schema';
