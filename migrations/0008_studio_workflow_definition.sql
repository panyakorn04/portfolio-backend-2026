-- Add a private, versioned workflow definition while preserving the public
-- string-array nodes projection used by execution-stage creation.
ALTER TABLE "StudioWorkflow"
ADD COLUMN IF NOT EXISTS "definition" JSONB;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'StudioWorkflow_definition_shape_check'
  ) THEN
    ALTER TABLE "StudioWorkflow"
    ADD CONSTRAINT "StudioWorkflow_definition_shape_check" CHECK (
      "definition" IS NULL OR (
        jsonb_typeof("definition") = 'object'
        AND "definition"->>'version' = '1'
        AND jsonb_typeof("definition"->'nodes') = 'array'
        AND jsonb_typeof("definition"->'edges') = 'array'
      )
    );
  END IF;
END
$$;

WITH workflow_nodes AS (
  SELECT
    workflow."id" AS workflow_id,
    node.label,
    node.ordinality - 1 AS position,
    'node-' || (node.ordinality - 1)::text AS node_id,
    CASE
      WHEN lower(node.label) LIKE '%schedule%' THEN 'schedule'
      WHEN lower(node.label) LIKE '%webhook%' THEN 'webhook'
      WHEN node.ordinality = 1 THEN 'manual'
      WHEN lower(node.label) = 'search' THEN 'search'
      WHEN lower(node.label) = 'analyze' THEN 'analyze'
      WHEN lower(node.label) = 'generate' THEN 'generate'
      WHEN lower(node.label) = 'extract' THEN 'extract'
      WHEN lower(node.label) = 'review' THEN 'review'
      WHEN lower(node.label) = 'approve' THEN 'approve'
      WHEN lower(node.label) = 'condition' THEN 'condition'
      WHEN lower(node.label) = 'route' THEN 'route'
      WHEN lower(node.label) = 'publish' THEN 'publish'
      WHEN lower(node.label) = 'notify' THEN 'notify'
      WHEN lower(node.label) = 'sync' THEN 'sync'
      WHEN lower(node.label) = 'export' THEN 'export'
      ELSE 'transform'
    END AS node_type
  FROM "StudioWorkflow" workflow
  CROSS JOIN LATERAL jsonb_array_elements_text(workflow."nodes") WITH ORDINALITY AS node(label, ordinality)
), definitions AS (
  SELECT
    workflow_id,
    jsonb_build_object(
      'version', 1,
      'nodes', jsonb_agg(
        jsonb_build_object(
          'id', node_id,
          'type', node_type,
          'kind', CASE
            WHEN node_type IN ('schedule', 'webhook', 'manual') THEN 'trigger'
            WHEN node_type IN ('review', 'approve', 'condition', 'route') THEN 'logic'
            WHEN node_type IN ('publish', 'notify', 'sync', 'export') THEN 'output'
            ELSE 'action'
          END,
          'label', label,
          'position', jsonb_build_object('x', position * 200, 'y', 0),
          'config', CASE
            WHEN node_type = 'schedule' THEN '{"enabled":true,"mode":"daily","timezone":"Asia/Bangkok","time":"09:00","misfirePolicy":"skip"}'::jsonb
            WHEN node_type = 'manual' THEN '{"enabled":true}'::jsonb
            WHEN node_type = 'webhook' THEN '{"enabled":true,"method":"POST","authMode":"none","responseMode":"immediate"}'::jsonb
            ELSE '{}'::jsonb
          END
        ) ORDER BY position
      ),
      'edges', COALESCE(
        jsonb_agg(
          jsonb_build_object(
            'id', 'edge-node-' || position::text || '-node-' || (position + 1)::text,
            'source', 'node-' || position::text,
            'target', 'node-' || (position + 1)::text
          ) ORDER BY position
        ) FILTER (WHERE position < (SELECT max(next_node.position) FROM workflow_nodes next_node WHERE next_node.workflow_id = workflow_nodes.workflow_id)),
        '[]'::jsonb
      )
    ) AS definition
  FROM workflow_nodes
  GROUP BY workflow_id
)
UPDATE "StudioWorkflow" workflow
SET "definition" = definitions.definition
FROM definitions
WHERE workflow."id" = definitions.workflow_id
  AND workflow."definition" IS NULL;
