-- Durable portfolio AI run claims prevent concurrent retries from generating twice.

CREATE TABLE IF NOT EXISTS "PortfolioChatRun" (
    "sessionId" TEXT NOT NULL REFERENCES "PortfolioChatSession"("id") ON DELETE CASCADE,
    "runId" TEXT NOT NULL,
    "visitorIdHash" TEXT NOT NULL,
    "userContent" TEXT NOT NULL,
    "status" TEXT NOT NULL DEFAULT 'generating',
    "leaseOwner" TEXT,
    "leaseUntil" TIMESTAMP,
    "assistantContent" TEXT,
    "modelName" TEXT,
    "createdAt" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY ("sessionId", "runId"),
    CONSTRAINT "PortfolioChatRun_runId_length_check" CHECK (char_length(trim("runId")) BETWEEN 1 AND 128),
    CONSTRAINT "PortfolioChatRun_userContent_length_check" CHECK (char_length(trim("userContent")) BETWEEN 1 AND 16384),
    CONSTRAINT "PortfolioChatRun_status_check" CHECK ("status" IN ('generating', 'completed')),
    CONSTRAINT "PortfolioChatRun_completion_check" CHECK (
        ("status" = 'generating' AND "leaseOwner" IS NOT NULL AND "leaseUntil" IS NOT NULL AND "assistantContent" IS NULL)
        OR
        ("status" = 'completed' AND "leaseOwner" IS NULL AND "leaseUntil" IS NULL AND "assistantContent" IS NOT NULL)
    )
);

ALTER TABLE "PortfolioChatRun" ENABLE ROW LEVEL SECURITY;

CREATE OR REPLACE FUNCTION "claimPortfolioChatRun"(
    "sessionId" TEXT,
    "visitorIdHash" TEXT,
    "runId" TEXT,
    "userContent" TEXT,
    "leaseOwner" TEXT,
    "leaseUntil" TIMESTAMP,
    "occurredAt" TIMESTAMP
) RETURNS TABLE (
    "outcome" TEXT,
    "assistantContent" TEXT,
    "modelName" TEXT
)
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    sessionRow "PortfolioChatSession"%ROWTYPE;
    runRow "PortfolioChatRun"%ROWTYPE;
    userRow "PortfolioChatMessage"%ROWTYPE;
    assistantRow "PortfolioChatMessage"%ROWTYPE;
BEGIN
    IF char_length(trim(COALESCE("runId", ''))) NOT BETWEEN 1 AND 128
       OR char_length(trim(COALESCE("userContent", ''))) NOT BETWEEN 1 AND 16384
       OR char_length(trim(COALESCE("leaseOwner", ''))) NOT BETWEEN 1 AND 128
       OR "leaseUntil" <= "occurredAt" THEN
        RAISE EXCEPTION 'invalid portfolio chat run claim';
    END IF;

    SELECT session.* INTO sessionRow
    FROM "PortfolioChatSession" AS session
    WHERE session."id" = "claimPortfolioChatRun"."sessionId"
      AND session."visitorIdHash" = "claimPortfolioChatRun"."visitorIdHash"
      AND session."expiresAt" > "claimPortfolioChatRun"."occurredAt"
    FOR UPDATE;

    IF NOT FOUND THEN
        RETURN QUERY SELECT 'not_found'::TEXT, NULL::TEXT, NULL::TEXT;
        RETURN;
    END IF;
    IF sessionRow."status" <> 'active' THEN
        RETURN QUERY SELECT 'state_conflict'::TEXT, NULL::TEXT, NULL::TEXT;
        RETURN;
    END IF;

    SELECT run.* INTO runRow
    FROM "PortfolioChatRun" AS run
    WHERE run."sessionId" = "claimPortfolioChatRun"."sessionId"
      AND run."runId" = trim("claimPortfolioChatRun"."runId")
    FOR UPDATE;

    IF FOUND THEN
        IF runRow."visitorIdHash" <> "claimPortfolioChatRun"."visitorIdHash"
           OR runRow."userContent" <> trim("claimPortfolioChatRun"."userContent") THEN
            RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::TEXT, NULL::TEXT;
        ELSIF runRow."status" = 'completed' THEN
            RETURN QUERY SELECT 'replayed'::TEXT, runRow."assistantContent", runRow."modelName";
        ELSIF runRow."leaseUntil" > "claimPortfolioChatRun"."occurredAt" THEN
            RETURN QUERY SELECT 'in_progress'::TEXT, NULL::TEXT, NULL::TEXT;
        ELSE
            UPDATE "PortfolioChatRun" AS run
            SET "leaseOwner" = trim("claimPortfolioChatRun"."leaseOwner"),
                "leaseUntil" = "claimPortfolioChatRun"."leaseUntil",
                "updatedAt" = "claimPortfolioChatRun"."occurredAt"
            WHERE run."sessionId" = "claimPortfolioChatRun"."sessionId"
              AND run."runId" = trim("claimPortfolioChatRun"."runId");
            RETURN QUERY SELECT 'claimed'::TEXT, NULL::TEXT, NULL::TEXT;
        END IF;
        RETURN;
    END IF;

    -- Bridge exchanges persisted by the pre-claim application version.
    SELECT message.* INTO userRow
    FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "claimPortfolioChatRun"."sessionId"
      AND message."runId" = trim("claimPortfolioChatRun"."runId")
      AND message."role" = 'user';
    SELECT message.* INTO assistantRow
    FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "claimPortfolioChatRun"."sessionId"
      AND message."runId" = trim("claimPortfolioChatRun"."runId")
      AND message."role" = 'assistant';

    IF userRow."id" IS NOT NULL OR assistantRow."id" IS NOT NULL THEN
        IF userRow."id" IS NOT NULL AND assistantRow."id" IS NOT NULL
           AND userRow."content" = trim("claimPortfolioChatRun"."userContent") THEN
            INSERT INTO "PortfolioChatRun" (
                "sessionId", "runId", "visitorIdHash", "userContent", "status",
                "assistantContent", "modelName", "createdAt", "updatedAt"
            ) VALUES (
                "sessionId", trim("runId"), "visitorIdHash", trim("userContent"), 'completed',
                assistantRow."content", assistantRow."metadata"->>'model', "occurredAt", "occurredAt"
            );
            RETURN QUERY SELECT 'replayed'::TEXT, assistantRow."content", assistantRow."metadata"->>'model';
        ELSE
            RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::TEXT, NULL::TEXT;
        END IF;
        RETURN;
    END IF;

    INSERT INTO "PortfolioChatRun" (
        "sessionId", "runId", "visitorIdHash", "userContent", "status",
        "leaseOwner", "leaseUntil", "createdAt", "updatedAt"
    ) VALUES (
        "sessionId", trim("runId"), "visitorIdHash", trim("userContent"), 'generating',
        trim("leaseOwner"), "leaseUntil", "occurredAt", "occurredAt"
    );
    RETURN QUERY SELECT 'claimed'::TEXT, NULL::TEXT, NULL::TEXT;
END;
$$;

CREATE OR REPLACE FUNCTION "completePortfolioChatRun"(
    "sessionId" TEXT,
    "visitorIdHash" TEXT,
    "runId" TEXT,
    "leaseOwner" TEXT,
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
    runRow "PortfolioChatRun"%ROWTYPE;
    userRow "PortfolioChatMessage"%ROWTYPE;
    assistantRow "PortfolioChatMessage"%ROWTYPE;
BEGIN
    IF char_length(trim(COALESCE("runId", ''))) NOT BETWEEN 1 AND 128
       OR char_length(trim(COALESCE("leaseOwner", ''))) NOT BETWEEN 1 AND 128
       OR char_length(trim(COALESCE("userContent", ''))) NOT BETWEEN 1 AND 16384
       OR char_length(trim(COALESCE("assistantContent", ''))) NOT BETWEEN 1 AND 16384
       OR char_length(trim(COALESCE("userMessageId", ''))) = 0
       OR char_length(trim(COALESCE("assistantMessageId", ''))) = 0
       OR "maxMessages" < 2 OR "maxMessages" > 500 THEN
        RAISE EXCEPTION 'invalid portfolio chat run completion';
    END IF;

    SELECT session.* INTO sessionRow
    FROM "PortfolioChatSession" AS session
    WHERE session."id" = "completePortfolioChatRun"."sessionId"
      AND session."visitorIdHash" = "completePortfolioChatRun"."visitorIdHash"
      AND session."expiresAt" > "completePortfolioChatRun"."occurredAt"
    FOR UPDATE;
    IF NOT FOUND THEN
        RETURN QUERY SELECT 'not_found'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;

    SELECT run.* INTO runRow
    FROM "PortfolioChatRun" AS run
    WHERE run."sessionId" = "completePortfolioChatRun"."sessionId"
      AND run."runId" = trim("completePortfolioChatRun"."runId")
    FOR UPDATE;
    IF NOT FOUND THEN
        RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;
    IF runRow."visitorIdHash" <> "completePortfolioChatRun"."visitorIdHash"
       OR runRow."userContent" <> trim("completePortfolioChatRun"."userContent") THEN
        RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;

    SELECT message.* INTO userRow FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "completePortfolioChatRun"."sessionId"
      AND message."runId" = trim("completePortfolioChatRun"."runId") AND message."role" = 'user';
    SELECT message.* INTO assistantRow FROM "PortfolioChatMessage" AS message
    WHERE message."sessionId" = "completePortfolioChatRun"."sessionId"
      AND message."runId" = trim("completePortfolioChatRun"."runId") AND message."role" = 'assistant';

    IF runRow."status" = 'completed' THEN
        IF userRow."id" IS NOT NULL AND assistantRow."id" IS NOT NULL
           AND assistantRow."content" = trim("completePortfolioChatRun"."assistantContent") THEN
            RETURN QUERY SELECT 'replayed'::TEXT, to_jsonb(userRow), to_jsonb(assistantRow);
        ELSE
            RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        END IF;
        RETURN;
    END IF;
    IF runRow."leaseOwner" <> trim("completePortfolioChatRun"."leaseOwner") THEN
        RETURN QUERY SELECT 'in_progress'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;
    IF sessionRow."status" <> 'active' THEN
        RETURN QUERY SELECT 'state_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;
    IF userRow."id" IS NOT NULL OR assistantRow."id" IS NOT NULL THEN
        RETURN QUERY SELECT 'idempotency_conflict'::TEXT, NULL::JSONB, NULL::JSONB;
        RETURN;
    END IF;

    INSERT INTO "PortfolioChatMessage" ("id", "sessionId", "role", "type", "content", "createdAt", "metadata", "runId")
    VALUES ("userMessageId", "sessionId", 'user', 'chat', trim("userContent"), "occurredAt",
            jsonb_build_object('source', 'portfolio-widget'), trim("runId")) RETURNING * INTO userRow;
    INSERT INTO "PortfolioChatMessage" ("id", "sessionId", "role", "type", "content", "createdAt", "metadata", "runId")
    VALUES ("assistantMessageId", "sessionId", 'assistant', 'chat', trim("assistantContent"), "occurredAt" + INTERVAL '1 microsecond',
            jsonb_build_object('source', 'portfolio-widget', 'model', "modelName"), trim("runId")) RETURNING * INTO assistantRow;

    UPDATE "PortfolioChatRun" AS run
    SET "status" = 'completed', "leaseOwner" = NULL, "leaseUntil" = NULL,
        "assistantContent" = trim("completePortfolioChatRun"."assistantContent"),
        "modelName" = "completePortfolioChatRun"."modelName", "updatedAt" = "occurredAt"
    WHERE run."sessionId" = "completePortfolioChatRun"."sessionId" AND run."runId" = trim("completePortfolioChatRun"."runId");
    UPDATE "PortfolioChatSession" AS session
    SET "updatedAt" = "occurredAt", "lastSeenAt" = "occurredAt",
        "expiresAt" = GREATEST(session."expiresAt", "completePortfolioChatRun"."expiresAt")
    WHERE session."id" = "completePortfolioChatRun"."sessionId";
    PERFORM "_prunePortfolioChatMessages"("sessionId", "maxMessages");

    RETURN QUERY SELECT 'inserted'::TEXT, to_jsonb(userRow), to_jsonb(assistantRow);
END;
$$;

CREATE OR REPLACE FUNCTION "releasePortfolioChatRun"(
    "sessionId" TEXT,
    "visitorIdHash" TEXT,
    "runId" TEXT,
    "leaseOwner" TEXT
) RETURNS BOOLEAN
LANGUAGE plpgsql
SECURITY INVOKER
SET search_path = public
AS $$
DECLARE
    deletedCount INTEGER;
BEGIN
    DELETE FROM "PortfolioChatRun" AS run
    WHERE run."sessionId" = "releasePortfolioChatRun"."sessionId"
      AND run."visitorIdHash" = "releasePortfolioChatRun"."visitorIdHash"
      AND run."runId" = trim("releasePortfolioChatRun"."runId")
      AND run."leaseOwner" = trim("releasePortfolioChatRun"."leaseOwner")
      AND run."status" = 'generating';
    GET DIAGNOSTICS deletedCount = ROW_COUNT;
    RETURN deletedCount = 1;
END;
$$;

REVOKE ALL ON TABLE "PortfolioChatRun" FROM PUBLIC, anon, authenticated;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE "PortfolioChatRun" TO service_role;

REVOKE ALL ON FUNCTION "claimPortfolioChatRun"(TEXT, TEXT, TEXT, TEXT, TEXT, TIMESTAMP, TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION "completePortfolioChatRun"(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TIMESTAMP, INTEGER, TIMESTAMP) FROM PUBLIC, anon, authenticated;
REVOKE ALL ON FUNCTION "releasePortfolioChatRun"(TEXT, TEXT, TEXT, TEXT) FROM PUBLIC, anon, authenticated;
GRANT EXECUTE ON FUNCTION "claimPortfolioChatRun"(TEXT, TEXT, TEXT, TEXT, TEXT, TIMESTAMP, TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "completePortfolioChatRun"(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TIMESTAMP, INTEGER, TIMESTAMP) TO service_role;
GRANT EXECUTE ON FUNCTION "releasePortfolioChatRun"(TEXT, TEXT, TEXT, TEXT) TO service_role;

NOTIFY pgrst, 'reload schema';
