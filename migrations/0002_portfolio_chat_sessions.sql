-- Portfolio public assistant anonymous chat sessions.
--
-- These tables persist panyakorn.com visitor chat history without storing the
-- raw browser visitor cookie. The backend stores only a server-side HMAC hash
-- of the anonymous visitor id.

CREATE TABLE IF NOT EXISTS "PortfolioChatSession" (
    "id"            TEXT PRIMARY KEY,
    "visitorIdHash" TEXT NOT NULL,
    "threadId"      TEXT NOT NULL UNIQUE,
    "locale"        TEXT NOT NULL DEFAULT 'en',
    "title"         TEXT,
    "createdAt"     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt"     TIMESTAMP NOT NULL,
    "lastSeenAt"    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "expiresAt"     TIMESTAMP NOT NULL
);

CREATE INDEX IF NOT EXISTS "PortfolioChatSession_visitorIdHash_lastSeenAt_idx"
    ON "PortfolioChatSession" ("visitorIdHash", "lastSeenAt" DESC);

CREATE INDEX IF NOT EXISTS "PortfolioChatSession_expiresAt_idx"
    ON "PortfolioChatSession" ("expiresAt");

CREATE TABLE IF NOT EXISTS "PortfolioChatMessage" (
    "id"        TEXT PRIMARY KEY,
    "sessionId" TEXT NOT NULL REFERENCES "PortfolioChatSession" ("id") ON DELETE CASCADE,
    "role"      TEXT NOT NULL,
    "content"   TEXT NOT NULL,
    "createdAt" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "metadata"  JSONB
);

CREATE INDEX IF NOT EXISTS "PortfolioChatMessage_sessionId_createdAt_idx"
    ON "PortfolioChatMessage" ("sessionId", "createdAt" ASC);
