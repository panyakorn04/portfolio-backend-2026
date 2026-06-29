-- Portfolio backend schema.
--
-- This MATCHES the existing Prisma-generated schema on Supabase so the go-zero
-- service can run against the live database without a data migration:
--   * Table names are PascalCase (quoted).
--   * Column names are camelCase (quoted).
--   * IDs are `text` (cuid) with NO database default — the application supplies
--     the ID on insert (see internal/model/id.go).
--   * Timestamps are `timestamp` (without time zone), as Prisma created them.
--
-- Use this only to bootstrap a brand-new database. The production database
-- already has these tables (created by Prisma migrations).

CREATE TABLE IF NOT EXISTS "User" (
    "id"           TEXT PRIMARY KEY,
    "email"        TEXT NOT NULL UNIQUE,
    "name"         TEXT,
    "passwordHash" TEXT NOT NULL,
    "role"         TEXT NOT NULL DEFAULT 'admin',
    "createdAt"    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"    TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS "AuthSession" (
    "id"               TEXT PRIMARY KEY,
    "sessionTokenHash" TEXT NOT NULL UNIQUE,
    "userId"           TEXT NOT NULL REFERENCES "User" ("id") ON DELETE CASCADE,
    "expiresAt"        TIMESTAMP NOT NULL,
    "createdAt"        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "lastSeenAt"       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "AuthSession_userId_expiresAt_idx"
    ON "AuthSession" ("userId", "expiresAt");

CREATE TABLE IF NOT EXISTS "ContactInquiry" (
    "id"           TEXT PRIMARY KEY,
    "name"         TEXT NOT NULL,
    "email"        TEXT NOT NULL,
    "company"      TEXT,
    "subject"      TEXT NOT NULL,
    "message"      TEXT NOT NULL,
    "locale"       TEXT NOT NULL DEFAULT 'en',
    "deliveryMode" TEXT NOT NULL DEFAULT 'database',
    "status"       TEXT NOT NULL DEFAULT 'new',
    "internalNote" TEXT,
    "handledAt"    TIMESTAMP,
    "createdAt"    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"    TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS "ContactInquiry_email_createdAt_idx"
    ON "ContactInquiry" ("email", "createdAt");
CREATE INDEX IF NOT EXISTS "ContactInquiry_locale_createdAt_idx"
    ON "ContactInquiry" ("locale", "createdAt");
CREATE INDEX IF NOT EXISTS "ContactInquiry_status_createdAt_idx"
    ON "ContactInquiry" ("status", "createdAt");

CREATE TABLE IF NOT EXISTS "ContactInquiryActivity" (
    "id"               TEXT PRIMARY KEY,
    "inquiryId"        TEXT NOT NULL REFERENCES "ContactInquiry" ("id") ON DELETE CASCADE,
    "actorType"        TEXT NOT NULL,
    "actorLabel"       TEXT NOT NULL,
    "eventType"        TEXT NOT NULL,
    "statusFrom"       TEXT,
    "statusTo"         TEXT,
    "internalNoteFrom" TEXT,
    "internalNoteTo"   TEXT,
    "createdAt"        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS "ContactInquiryActivity_inquiryId_createdAt_idx"
    ON "ContactInquiryActivity" ("inquiryId", "createdAt");

CREATE TABLE IF NOT EXISTS "Article" (
    "id"          TEXT PRIMARY KEY,
    "slug"        TEXT NOT NULL UNIQUE,
    "category"    TEXT NOT NULL,
    "status"      TEXT NOT NULL DEFAULT 'draft',
    "publishedAt" TIMESTAMP,
    "createdAt"   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"   TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS "Article_status_publishedAt_idx"
    ON "Article" ("status", "publishedAt");

CREATE TABLE IF NOT EXISTS "ArticleTranslation" (
    "id"          TEXT PRIMARY KEY,
    "articleId"   TEXT NOT NULL REFERENCES "Article" ("id") ON DELETE CASCADE,
    "locale"      TEXT NOT NULL,
    "title"       TEXT NOT NULL,
    "summary"     TEXT NOT NULL,
    "lead"        TEXT NOT NULL,
    "readingTime" TEXT NOT NULL,
    "sections"    JSONB NOT NULL,
    UNIQUE ("articleId", "locale")
);
