-- Admin chat support for portfolio chat sessions
-- Allows admin to take over conversations and respond to visitors

ALTER TABLE "PortfolioChatSession"
ADD COLUMN IF NOT EXISTS "status" TEXT NOT NULL DEFAULT 'active';

-- status values:
-- 'active'        — AI auto-reply mode (default)
-- 'pending_human' — visitor requested human contact
-- 'human'         — admin has taken over the conversation

ALTER TABLE "PortfolioChatMessage"
ADD COLUMN IF NOT EXISTS "type" TEXT NOT NULL DEFAULT 'chat';
-- type values:
-- 'chat'           — regular chat message (user or AI)
-- 'request_human'  — visitor requested human contact (system event)
-- 'human_takeover' — admin took over conversation (system event)
