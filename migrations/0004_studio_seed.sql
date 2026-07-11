-- Seed the production Studio portfolio demo after 0003_studio.sql.
-- Idempotent: safe to run more than once.

INSERT INTO "StudioWorkflow" ("id", "name", "description", "category", "status", "runs", "success", "nodes", "updatedAt")
VALUES
  ('wf-content', 'Content intelligence pipeline', 'Discover, analyze, generate, approve, and publish content.', 'Content', 'active', 1284, 98.4, '["Discover","Analyze","Generate","Approval","Publish"]'::jsonb, CURRENT_TIMESTAMP),
  ('wf-research', 'Competitive research brief', 'Turn multiple sources into a cited bilingual market brief.', 'Research', 'active', 486, 96.8, '["Search","Extract","Synthesize","Review"]'::jsonb, CURRENT_TIMESTAMP),
  ('wf-meeting', 'Meeting action center', 'Summarize meetings, identify owners, and sync action items.', 'Operations', 'draft', 72, 94.1, '["Transcript","Summarize","Assign","Sync"]'::jsonb, CURRENT_TIMESTAMP)
ON CONFLICT ("id") DO UPDATE SET
  "name" = EXCLUDED."name",
  "description" = EXCLUDED."description",
  "category" = EXCLUDED."category",
  "status" = EXCLUDED."status",
  "runs" = EXCLUDED."runs",
  "success" = EXCLUDED."success",
  "nodes" = EXCLUDED."nodes",
  "updatedAt" = CURRENT_TIMESTAMP;

INSERT INTO "StudioExecution" ("id", "workflowId", "workflow", "status", "startedAt", "durationMs", "cost", "updatedAt")
VALUES
  ('RUN-2841', 'wf-content', 'Content intelligence pipeline', 'running', CURRENT_TIMESTAMP - INTERVAL '2 minutes', 102000, 0.18, CURRENT_TIMESTAMP),
  ('RUN-2840', 'wf-research', 'Competitive research brief', 'completed', CURRENT_TIMESTAMP - INTERVAL '12 minutes', 138000, 0.12, CURRENT_TIMESTAMP),
  ('RUN-2839', 'wf-meeting', 'Meeting action center', 'waiting', CURRENT_TIMESTAMP - INTERVAL '26 minutes', 48000, 0.06, CURRENT_TIMESTAMP),
  ('RUN-2838', 'wf-content', 'Content intelligence pipeline', 'failed', CURRENT_TIMESTAMP - INTERVAL '1 hour', 34000, 0.03, CURRENT_TIMESTAMP),
  ('RUN-2837', 'wf-content', 'Content intelligence pipeline', 'completed', CURRENT_TIMESTAMP - INTERVAL '2 hours', 126000, 0.16, CURRENT_TIMESTAMP)
ON CONFLICT ("id") DO UPDATE SET
  "workflowId" = EXCLUDED."workflowId",
  "workflow" = EXCLUDED."workflow",
  "status" = EXCLUDED."status",
  "startedAt" = EXCLUDED."startedAt",
  "durationMs" = EXCLUDED."durationMs",
  "cost" = EXCLUDED."cost",
  "updatedAt" = CURRENT_TIMESTAMP;
