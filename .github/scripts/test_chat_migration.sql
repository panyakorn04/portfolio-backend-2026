\set ON_ERROR_STOP on

INSERT INTO "PortfolioChatSession" (
  "id", "visitorIdHash", "threadId", "locale", "updatedAt", "lastSeenAt", "expiresAt", "status"
) VALUES
  ('rpc-exchange', 'visitor-a', 'thread-a', 'en', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + INTERVAL '1 day', 'active'),
  ('rpc-run', 'visitor-run', 'thread-run', 'en', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + INTERVAL '1 day', 'active'),
  ('rpc-handoff', 'visitor-b', 'thread-b', 'en', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP + INTERVAL '1 day', 'active'),
  ('rpc-expired', 'visitor-c', 'thread-c', 'en', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP - INTERVAL '1 day', 'active');

SET ROLE service_role;

DO $$
DECLARE
  result TEXT;
  message_count INTEGER;
  sessions_deleted BIGINT;
  replayed_content TEXT;
BEGIN
  SELECT "outcome" INTO result FROM "persistPortfolioChatExchange"(
    'rpc-exchange', 'visitor-a', 'run-a', 'user-a', 'assistant-a',
    'Hello', 'Hi there', 'test-model', (CURRENT_TIMESTAMP + INTERVAL '2 days')::timestamp, 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'inserted' THEN RAISE EXCEPTION 'exchange insert outcome: %', result; END IF;

  SELECT "outcome" INTO result FROM "persistPortfolioChatExchange"(
    'rpc-exchange', 'visitor-a', 'run-a', 'ignored-user-id', 'ignored-assistant-id',
    'Hello', 'Hi there', 'test-model', (CURRENT_TIMESTAMP + INTERVAL '2 days')::timestamp, 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'replayed' THEN RAISE EXCEPTION 'exchange replay outcome: %', result; END IF;

  SELECT "outcome" INTO result FROM "persistPortfolioChatExchange"(
    'rpc-exchange', 'visitor-a', 'run-a', 'ignored-user-id-2', 'ignored-assistant-id-2',
    'Changed input', 'Hi there', 'test-model', (CURRENT_TIMESTAMP + INTERVAL '2 days')::timestamp, 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'idempotency_conflict' THEN RAISE EXCEPTION 'exchange conflict outcome: %', result; END IF;

  SELECT count(*) INTO message_count FROM "PortfolioChatMessage" WHERE "sessionId" = 'rpc-exchange';
  IF message_count <> 2 THEN RAISE EXCEPTION 'exchange message count: %', message_count; END IF;

  SELECT "outcome" INTO result FROM "claimPortfolioChatRun"(
    'rpc-run', 'visitor-run', 'run-claimed', 'Question', 'owner-a',
    (CURRENT_TIMESTAMP + INTERVAL '10 minutes')::timestamp, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'claimed' THEN RAISE EXCEPTION 'run claim outcome: %', result; END IF;

  SELECT "outcome" INTO result FROM "claimPortfolioChatRun"(
    'rpc-run', 'visitor-run', 'run-claimed', 'Question', 'owner-b',
    (CURRENT_TIMESTAMP + INTERVAL '10 minutes')::timestamp, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'in_progress' THEN RAISE EXCEPTION 'concurrent run claim outcome: %', result; END IF;

  SELECT "outcome" INTO result FROM "completePortfolioChatRun"(
    'rpc-run', 'visitor-run', 'run-claimed', 'owner-a', 'run-user', 'run-assistant',
    'Question', 'Durable answer', 'test-model', (CURRENT_TIMESTAMP + INTERVAL '2 days')::timestamp, 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'inserted' THEN RAISE EXCEPTION 'run completion outcome: %', result; END IF;

  SELECT "outcome", "assistantContent" INTO result, replayed_content FROM "claimPortfolioChatRun"(
    'rpc-run', 'visitor-run', 'run-claimed', 'Question', 'owner-c',
    (CURRENT_TIMESTAMP + INTERVAL '10 minutes')::timestamp, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'replayed' OR replayed_content <> 'Durable answer' THEN
    RAISE EXCEPTION 'run replay outcome/content: %/%', result, replayed_content;
  END IF;

  SELECT "outcome" INTO result FROM "claimPortfolioChatRun"(
    'rpc-run', 'visitor-run', 'run-claimed', 'Different question', 'owner-d',
    (CURRENT_TIMESTAMP + INTERVAL '10 minutes')::timestamp, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'idempotency_conflict' THEN RAISE EXCEPTION 'run conflict outcome: %', result; END IF;

  SELECT "outcome" INTO result FROM "claimPortfolioChatRun"(
    'rpc-run', 'visitor-run', 'run-release', 'Release me', 'owner-release',
    (CURRENT_TIMESTAMP + INTERVAL '10 minutes')::timestamp, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'claimed' OR NOT "releasePortfolioChatRun"('rpc-run', 'visitor-run', 'run-release', 'owner-release') THEN
    RAISE EXCEPTION 'run release failed';
  END IF;

  SELECT "outcome" INTO result FROM "requestPortfolioChatHuman"(
    'rpc-handoff', 'visitor-b', 'handoff-a', 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'updated' THEN RAISE EXCEPTION 'handoff update outcome: %', result; END IF;

  SELECT "outcome" INTO result FROM "requestPortfolioChatHuman"(
    'rpc-handoff', 'visitor-b', 'ignored-handoff-id', 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'replayed' THEN RAISE EXCEPTION 'handoff replay outcome: %', result; END IF;

  SELECT count(*) INTO message_count FROM "PortfolioChatMessage"
  WHERE "sessionId" = 'rpc-handoff' AND "type" = 'request_human';
  IF message_count <> 1 THEN RAISE EXCEPTION 'handoff event count: %', message_count; END IF;

  SELECT "outcome" INTO result FROM "takeoverAndReplyPortfolioChat"(
    'rpc-handoff', 'takeover-a', 'reply-a', 'A human reply', 'Admin', 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'replied' THEN RAISE EXCEPTION 'admin reply outcome: %', result; END IF;
  IF (SELECT "status" FROM "PortfolioChatSession" WHERE "id" = 'rpc-handoff') <> 'human' THEN
    RAISE EXCEPTION 'admin takeover did not transition session';
  END IF;

  SELECT "outcome" INTO result FROM "takeoverAndReplyPortfolioChat"(
    'rpc-handoff', 'takeover-a', 'reply-a', 'A human reply', 'Admin', 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'replayed' THEN RAISE EXCEPTION 'admin reply replay outcome: %', result; END IF;
  SELECT count(*) INTO message_count FROM "PortfolioChatMessage"
  WHERE "sessionId" = 'rpc-handoff' AND "id" = 'reply-a';
  IF message_count <> 1 THEN RAISE EXCEPTION 'admin reply replay count: %', message_count; END IF;

  SELECT "outcome" INTO result FROM "takeoverAndReplyPortfolioChat"(
    'rpc-handoff', 'takeover-a', 'reply-a', 'Different reply', 'Admin', 50, CURRENT_TIMESTAMP::timestamp
  );
  IF result <> 'idempotency_conflict' THEN RAISE EXCEPTION 'admin reply conflict outcome: %', result; END IF;

  SELECT "sessionsDeleted" INTO sessions_deleted
  FROM "prunePortfolioChatRetention"(50, CURRENT_TIMESTAMP::timestamp, 100);
  IF sessions_deleted <> 1 THEN RAISE EXCEPTION 'retention deleted sessions: %', sessions_deleted; END IF;
  IF EXISTS (SELECT 1 FROM "PortfolioChatSession" WHERE "id" = 'rpc-expired') THEN
    RAISE EXCEPTION 'expired session remains after retention';
  END IF;
END;
$$;

RESET ROLE;

INSERT INTO "PortfolioMigration" ("version", "filename", "checksum")
VALUES ('9999', '9999_smoke.sql', repeat('a', 64));

DO $$
BEGIN
  BEGIN
    UPDATE "PortfolioMigration" SET "checksum" = repeat('b', 64) WHERE "version" = '9999';
    RAISE EXCEPTION 'ledger update unexpectedly succeeded';
  EXCEPTION WHEN raise_exception THEN
    IF SQLERRM = 'ledger update unexpectedly succeeded' THEN RAISE; END IF;
  END;
  BEGIN
    DELETE FROM "PortfolioMigration" WHERE "version" = '9999';
    RAISE EXCEPTION 'ledger delete unexpectedly succeeded';
  EXCEPTION WHEN raise_exception THEN
    IF SQLERRM = 'ledger delete unexpectedly succeeded' THEN RAISE; END IF;
  END;
END;
$$;

SELECT 'transactional chat and migration ledger smoke tests passed' AS result;
