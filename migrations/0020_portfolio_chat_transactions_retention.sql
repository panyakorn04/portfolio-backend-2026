-- Transactional portfolio chat mutations, idempotent AI runs, and physical retention.

ALTER TABLE "PortfolioChatMessage"
    ADD COLUMN IF NOT EXISTS "runId" TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS "PortfolioChatMessage_sessionId_runId_role_unique_idx"
    ON "PortfolioChatMessage" ("sessionId", "runId", "role")
    WHERE "runId" IS NOT NULL;

CREATE INDEX IF NOT EXISTS "PortfolioChatMessage_sessionId_createdAt_id_idx"
    ON "PortfolioChatMessage" ("sessionId", "createdAt" DESC, "id" DESC);

DO $$
BEGIN
    ALTER TABLE "PortfolioChatSession"
        ADD CONSTRAINT "PortfolioChatSession_status_check"
        CHECK ("status" IN ('active', 'pending_human', 'human')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE "PortfolioChatMessage"
        ADD CONSTRAINT "PortfolioChatMessage_role_check"
        CHECK ("role" IN ('user', 'assistant', 'system')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE "PortfolioChatMessage"
        ADD CONSTRAINT "PortfolioChatMessage_type_check"
        CHECK ("type" IN ('chat', 'request_human', 'human_takeover')) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE "PortfolioChatMessage"
        ADD CONSTRAINT "PortfolioChatMessage_content_length_check"
        CHECK (char_length("content") BETWEEN 1 AND 16384) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$
BEGIN
    ALTER TABLE "PortfolioChatMessage"
        ADD CONSTRAINT "PortfolioChatMessage_runId_length_check"
        CHECK ("runId" IS NULL OR char_length("runId") BETWEEN 1 AND 128) NOT VALID;
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE OR REPLACE FUNCTION "_prunePortfolioChatMessages"(
    "sessionId" TEXT,
    "maxMessages" INTEGER
) RETURNS INTEGER
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    "deletedCount" INTEGER;
BEGIN
    IF "maxMessages" < 2 OR "maxMessages" > 500 THEN
        RAISE EXCEPTION 'maxMessages must be between 2 and 500';
    END IF;

    WITH "obsolete" AS (
        SELECT message."id"
        FROM "PortfolioChatMessage" AS message
        WHERE message."sessionId" = "_prunePortfolioChatMessages"."sessionId"
        ORDER BY message."createdAt" DESC, message."id" DESC
        OFFSET "maxMessages"
    )
    DELETE FROM "PortfolioChatMessage" AS message
    USING "obsolete"
    WHERE message."id" = "obsolete"."id";

    GET DIAGNOSTICS "deletedCount" = ROW_COUNT;
    RETURN "deletedCount";
END;
$$;

CREATE OR REPLACE FUNCTION "persistPortfolioChatExchange"(
    "sessionId" TEXT,
    "visitorIdHash" TEXT,
    "runId" TEXT,
    "userMessageId" TEXT,
    "assistantMessageId" TEXT,
    "userContent" TEXT,
    "assistantContent" TEXT,
    "modelName" TEXT,
    "expiresAt" TIMESTAMP,
    "maxMessages" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS TABLE (
    "outcome" TEXT,
    "userMessage" JSONB,
    "assistantMessage" JSONB
)
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    sessionRow "PortfolioChatSession"%ROWTYPE;
    userRow "PortfolioChatMessage"%ROWTYPE;
    assistantRow "PortfolioChatMessage"%ROWTYPE;
    userFound BOOLEAN := FALSE;
    assistantFound BOOLEAN := FALSE;
BEGIN
    IF char_length(trim(COALESCE("runId", ''))) NOT BETWEEN 1 AND 128
       OR char_length(trim(COALESCE("userContent", ''))) NOT BETWEEN 1 AND 16384
       OR char_length(trim(COALESCE("assistantContent", ''))) NOT BETWEEN 1 AND 16384
       OR char_length(trim(COALESCE("userMessageId", ''))) = 0
       OR char_length(trim(COALESCE("assistantMessageId", ''))) = 0
       OR "maxMessages" < 2 OR "maxMessages" > 500 THEN
        RAISE EXCEPTION 'invalid portfolio chat exchange input';
    END IF;

    SELECT session.* INTO sessionRow
    FROM "PortfolioChatSession" AS session
    WHERE session."id" = "persistPortfolioChatExchange"."sessionId"
      AND session."visitorIdHash" = "persistPortfolioChatExchange"."visitorIdHash"
      AND session."expiresAt" > "persistPortfolioChatExchange"."occurredAt"
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN QUERY SELECT 'not_found'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;

    SELECT message.* INTO userRow
    FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "persistPortfolioChatExchange"."sessionId"
      AND message."runId" = "persistPortfolioChatExchange"."runId"
      AND message."role" = 'user';
    userFound := FOUND;

    SELECT message.* INTO assistantRow
    FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "persistPortfolioChatExchange"."sessionId"
      AND message."runId" = "persistPortfolioChatExchange"."runId"
      AND message."role" = 'assistant';
    assistantFound := FOUND;

    IF userFound OR assistantFound THEN
        IF userFound AND assistantFound
           AND userRow."content" = trim("userContent")
           AND assistantRow."content" = trim("assistantContent") THEN
            RETURN QUERY SELECT 'replayed'::TEXT, to_jsonb(userRow), to_jsonb(assistantRow);
        ELSE
            RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        END IF;
        RETURN;
    END IF;

    IF sessionRow."status" <> 'active' THEN
        RETURN QUERY SELECT 'state_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;

    INSERT INTO "PortfolioChatMessage" (
        "id", "sessionId", "role", "type", "content", "createdAt", "metadata", "runId"
    ) VALUES (
        "userMessageId", "sessionId", 'user', 'chat', trim("userContent"), "occurredAt",
        jsonb_build_object('source', 'portfolio-widget'), "runId"
    ) RETURNING * INTO userRow;

    INSERT INTO "PortfolioChatMessage" (
        "id", "sessionId", "role", "type", "content", "createdAt", "metadata", "runId"
    ) VALUES (
        "assistantMessageId", "sessionId", 'assistant', 'chat', trim("assistantContent"),
        "occurredAt" + INTERVAL '1 microsecond',
        jsonb_build_object('source', 'portfolio-widget', 'model', "modelName"), "runId"
    ) RETURNING * INTO assistantRow;

    UPDATE "PortfolioChatSession" AS session
    SET "updatedAt" = "occurredAt",
        "lastSeenAt" = "occurredAt",
        "expiresAt" = GREATEST(session."expiresAt", "persistPortfolioChatExchange"."expiresAt")
    WHERE session."id" = "persistPortfolioChatExchange"."sessionId";

    PERFORM "_prunePortfolioChatMessages"("sessionId", "maxMessages");

    RETURN QUERY SELECT 'inserted'::TEXT, to_jsonb(userRow), to_jsonb(assistantRow);
END;
$$;

CREATE OR REPLACE FUNCTION "requestPortfolioChatHuman"(
    "sessionId" TEXT,
    "visitorIdHash" TEXT,
    "eventMessageId" TEXT,
    "maxMessages" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS TABLE (
    "outcome" TEXT,
    "status" TEXT,
    "eventMessage" JSONB
)
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    sessionRow "PortfolioChatSession"%ROWTYPE;
    eventRow "PortfolioChatMessage"%ROWTYPE;
BEGIN
    IF char_length(trim(COALESCE("eventMessageId", ''))) = 0
       OR "maxMessages" < 2 OR "maxMessages" > 500 THEN
        RAISE EXCEPTION 'invalid portfolio chat handoff input';
    END IF;

    SELECT session.* INTO sessionRow
    FROM "PortfolioChatSession" AS session
    WHERE session."id" = "requestPortfolioChatHuman"."sessionId"
      AND session."visitorIdHash" = "requestPortfolioChatHuman"."visitorIdHash"
      AND session."expiresAt" > "requestPortfolioChatHuman"."occurredAt"
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN QUERY SELECT 'not_found'::TEXT, NULL::TEXT, NULL::JSONB;
        RETURN;
    END IF;

    IF sessionRow."status" = 'human' THEN
        RETURN QUERY SELECT 'state_conflict'::TEXT, 'human'::TEXT, NULL::JSONB;
        RETURN;
    END IF;

    SELECT message.* INTO eventRow
    FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "requestPortfolioChatHuman"."sessionId"
      AND message."type" = 'request_human'
    ORDER BY message."createdAt" DESC, message."id" DESC
    LIMIT 1;

    IF sessionRow."status" = 'pending_human' AND FOUND THEN
        RETURN QUERY SELECT 'replayed'::TEXT, 'pending_human'::TEXT, to_jsonb(eventRow);
        RETURN;
    END IF;

    IF sessionRow."status" = 'active' THEN
        UPDATE "PortfolioChatSession" AS session
        SET "status" = 'pending_human', "updatedAt" = "occurredAt", "lastSeenAt" = "occurredAt"
        WHERE session."id" = "requestPortfolioChatHuman"."sessionId";
    END IF;

    INSERT INTO "PortfolioChatMessage" (
        "id", "sessionId", "role", "type", "content", "createdAt", "metadata"
    ) VALUES (
        "eventMessageId", "sessionId", 'system', 'request_human',
        'Visitor requested a human response.', "occurredAt",
        jsonb_build_object('source', 'portfolio-widget')
    ) RETURNING * INTO eventRow;

    PERFORM "_prunePortfolioChatMessages"("sessionId", "maxMessages");

    RETURN QUERY SELECT
        CASE WHEN sessionRow."status" = 'active' THEN 'updated' ELSE 'healed' END::TEXT,
        'pending_human'::TEXT,
        to_jsonb(eventRow);
END;
$$;

CREATE OR REPLACE FUNCTION "takeoverAndReplyPortfolioChat"(
    "sessionId" TEXT,
    "takeoverMessageId" TEXT,
    "replyMessageId" TEXT,
    "replyContent" TEXT,
    "adminName" TEXT,
    "maxMessages" INTEGER,
    "occurredAt" TIMESTAMP
) RETURNS TABLE (
    "outcome" TEXT,
    "status" TEXT,
    "replyMessage" JSONB
)
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    sessionRow "PortfolioChatSession"%ROWTYPE;
    replyRow "PortfolioChatMessage"%ROWTYPE;
BEGIN
    IF char_length(trim(COALESCE("replyContent", ''))) NOT BETWEEN 1 AND 16384
       OR char_length(trim(COALESCE("replyMessageId", ''))) = 0
       OR char_length(trim(COALESCE("takeoverMessageId", ''))) = 0
       OR "maxMessages" < 2 OR "maxMessages" > 500 THEN
        RAISE EXCEPTION 'invalid portfolio chat reply input';
    END IF;

    SELECT session.* INTO sessionRow
    FROM "PortfolioChatSession" AS session
    WHERE session."id" = "takeoverAndReplyPortfolioChat"."sessionId"
      AND session."expiresAt" > "takeoverAndReplyPortfolioChat"."occurredAt"
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN QUERY SELECT 'not_found'::TEXT, NULL::TEXT, NULL::JSONB;
        RETURN;
    END IF;

    SELECT message.* INTO replyRow
    FROM "PortfolioChatMessage" AS message
    WHERE message."id" = "takeoverAndReplyPortfolioChat"."replyMessageId";

    IF FOUND THEN
        IF replyRow."sessionId" = "takeoverAndReplyPortfolioChat"."sessionId"
           AND replyRow."role" = 'assistant'
           AND replyRow."type" = 'chat'
           AND replyRow."content" = trim("replyContent") THEN
            RETURN QUERY SELECT 'replayed'::TEXT, sessionRow."status", to_jsonb(replyRow);
        ELSE
            RETURN QUERY SELECT 'idempotency_conflict'::TEXT, sessionRow."status", NULL::JSONB;
        END IF;
        RETURN;
    END IF;

    IF sessionRow."status" <> 'human' THEN
        UPDATE "PortfolioChatSession" AS session
        SET "status" = 'human', "updatedAt" = "occurredAt", "lastSeenAt" = "occurredAt"
        WHERE session."id" = "takeoverAndReplyPortfolioChat"."sessionId";

        INSERT INTO "PortfolioChatMessage" (
            "id", "sessionId", "role", "type", "content", "createdAt", "metadata"
        ) VALUES (
            "takeoverMessageId", "sessionId", 'system', 'human_takeover',
            'A human has joined the conversation.', "occurredAt",
            jsonb_build_object('source', 'admin', 'adminName', trim(COALESCE("adminName", '')))
        );
    END IF;

    INSERT INTO "PortfolioChatMessage" (
        "id", "sessionId", "role", "type", "content", "createdAt", "metadata"
    ) VALUES (
        "replyMessageId", "sessionId", 'assistant', 'chat', trim("replyContent"),
        "occurredAt" + INTERVAL '1 microsecond',
        jsonb_build_object('source', 'admin', 'adminName', trim(COALESCE("adminName", '')))
    ) RETURNING * INTO replyRow;

    UPDATE "PortfolioChatSession" AS session
    SET "updatedAt" = "occurredAt", "lastSeenAt" = "occurredAt"
    WHERE session."id" = "takeoverAndReplyPortfolioChat"."sessionId";

    PERFORM "_prunePortfolioChatMessages"("sessionId", "maxMessages");

    RETURN QUERY SELECT 'replied'::TEXT, 'human'::TEXT, to_jsonb(replyRow);
END;
$$;

CREATE OR REPLACE FUNCTION "prunePortfolioChatRetention"(
    "maxMessages" INTEGER,
    "expiredBefore" TIMESTAMP,
    "batchSize" INTEGER
) RETURNS TABLE (
    "messagesDeleted" BIGINT,
    "sessionsDeleted" BIGINT
)
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    deletedMessages BIGINT := 0;
    deletedSessions BIGINT := 0;
BEGIN
    IF "maxMessages" < 2 OR "maxMessages" > 500
       OR "batchSize" < 1 OR "batchSize" > 1000 THEN
        RAISE EXCEPTION 'invalid portfolio chat retention input';
    END IF;

    WITH expired AS (
        SELECT session."id"
        FROM "PortfolioChatSession" AS session
        WHERE session."expiresAt" <= "expiredBefore"
        ORDER BY session."expiresAt", session."id"
        FOR UPDATE SKIP LOCKED
        LIMIT "batchSize"
    ), removed AS (
        DELETE FROM "PortfolioChatSession" AS session
        USING expired
        WHERE session."id" = expired."id"
        RETURNING session."id"
    )
    SELECT count(*) INTO deletedSessions FROM removed;

    WITH oversized AS (
        SELECT session."id"
        FROM "PortfolioChatSession" AS session
        WHERE (SELECT count(*) FROM "PortfolioChatMessage" AS message WHERE message."sessionId" = session."id") > "maxMessages"
        ORDER BY session."updatedAt", session."id"
        FOR UPDATE SKIP LOCKED
        LIMIT "batchSize"
    ), ranked AS (
        SELECT message."id",
               row_number() OVER (
                   PARTITION BY message."sessionId"
                   ORDER BY message."createdAt" DESC, message."id" DESC
               ) AS position
        FROM "PortfolioChatMessage" AS message
        JOIN oversized ON oversized."id" = message."sessionId"
    ), removed AS (
        DELETE FROM "PortfolioChatMessage" AS message
        USING ranked
        WHERE message."id" = ranked."id"
          AND ranked.position > "maxMessages"
        RETURNING message."id"
    )
    SELECT count(*) INTO deletedMessages FROM removed;

    RETURN QUERY SELECT deletedMessages, deletedSessions;
END;
$$;

REVOKE ALL ON FUNCTION "_prunePortfolioChatMessages"(TEXT, INTEGER) FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION "persistPortfolioChatExchange"(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TIMESTAMP, INTEGER, TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION "requestPortfolioChatHuman"(TEXT, TEXT, TEXT, INTEGER, TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION "takeoverAndReplyPortfolioChat"(TEXT, TEXT, TEXT, TEXT, TEXT, INTEGER, TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION "prunePortfolioChatRetention"(INTEGER, TIMESTAMP, INTEGER) FROM PUBLIC, anon, authenticated;

GRANT EXECUTE ON FUNCTION "_prunePortfolioChatMessages"(TEXT, INTEGER) TO service_role;
GRANT EXECUTE ON FUNCTION "persistPortfolioChatExchange"(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TIMESTAMP, INTEGER, TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "requestPortfolioChatHuman"(TEXT, TEXT, TEXT, INTEGER, TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "takeoverAndReplyPortfolioChat"(TEXT, TEXT, TEXT, TEXT, TEXT, INTEGER, TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "prunePortfolioChatRetention"(INTEGER, TIMESTAMP, INTEGER) TO service_role;

NOTIFY pgrst, 'reload schema';
