-- Add execution creation to the immutable Studio audit action allowlist.
ALTER TABLE "StudioAuditLog" DROP CONSTRAINT IF EXISTS "StudioAuditLog_action_check";
ALTER TABLE "StudioAuditLog" ADD CONSTRAINT "StudioAuditLog_action_check"
CHECK ("action" IN ('workflow.create', 'workflow.update', 'execution.create', 'execution.pause', 'execution.retry', 'execution.cancel', 'execution.approve'));
