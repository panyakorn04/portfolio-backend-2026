-- Restrict every application table to the backend service role.
--
-- Runtime persistence is intentionally server-side through PostgREST using the
-- service_role credential. Browser clients must use the portfolio API and must
-- never query these tables directly with Supabase anon/authenticated roles.

ALTER TABLE "User" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "AuthSession" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "ContactInquiry" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "ContactInquiryActivity" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "Article" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "ArticleTranslation" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "PortfolioChatSession" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "PortfolioChatMessage" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "StudioWorkflow" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "StudioExecution" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "StudioExecutionStage" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "StudioAuditLog" ENABLE ROW LEVEL SECURITY;
ALTER TABLE "StudioCredential" ENABLE ROW LEVEL SECURITY;

REVOKE ALL ON TABLE
    "User",
    "AuthSession",
    "ContactInquiry",
    "ContactInquiryActivity",
    "Article",
    "ArticleTranslation",
    "PortfolioChatSession",
    "PortfolioChatMessage",
    "StudioWorkflow",
    "StudioExecution",
    "StudioExecutionStage",
    "StudioAuditLog",
    "StudioCredential"
FROM PUBLIC, anon, authenticated;

GRANT ALL ON TABLE
    "User",
    "AuthSession",
    "ContactInquiry",
    "ContactInquiryActivity",
    "Article",
    "ArticleTranslation",
    "PortfolioChatSession",
    "PortfolioChatMessage",
    "StudioWorkflow",
    "StudioExecution",
    "StudioExecutionStage",
    "StudioAuditLog",
    "StudioCredential"
TO service_role;

-- Make backend-only access the default for tables added by future migrations.
-- A future direct-client table must opt in explicitly with reviewed RLS policies.
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public
    REVOKE ALL ON TABLES FROM PUBLIC, anon, authenticated;
ALTER DEFAULT PRIVILEGES FOR ROLE postgres IN SCHEMA public
    GRANT ALL ON TABLES TO service_role;
