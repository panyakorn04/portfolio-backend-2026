-- Enforce the public contact API's bounded input contract at the database edge.
-- NOT VALID avoids rejecting the migration because of historical rows while the
-- constraints still apply immediately to every new insert/update.

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ContactInquiry_name_length_check'
          AND conrelid = '"ContactInquiry"'::regclass
    ) THEN
        ALTER TABLE "ContactInquiry"
            ADD CONSTRAINT "ContactInquiry_name_length_check"
            CHECK (char_length("name") BETWEEN 2 AND 120) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ContactInquiry_email_length_check'
          AND conrelid = '"ContactInquiry"'::regclass
    ) THEN
        ALTER TABLE "ContactInquiry"
            ADD CONSTRAINT "ContactInquiry_email_length_check"
            CHECK (char_length("email") BETWEEN 3 AND 254) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ContactInquiry_company_length_check'
          AND conrelid = '"ContactInquiry"'::regclass
    ) THEN
        ALTER TABLE "ContactInquiry"
            ADD CONSTRAINT "ContactInquiry_company_length_check"
            CHECK ("company" IS NULL OR char_length("company") <= 160) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ContactInquiry_subject_length_check'
          AND conrelid = '"ContactInquiry"'::regclass
    ) THEN
        ALTER TABLE "ContactInquiry"
            ADD CONSTRAINT "ContactInquiry_subject_length_check"
            CHECK (char_length("subject") BETWEEN 3 AND 200) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ContactInquiry_message_length_check'
          AND conrelid = '"ContactInquiry"'::regclass
    ) THEN
        ALTER TABLE "ContactInquiry"
            ADD CONSTRAINT "ContactInquiry_message_length_check"
            CHECK (char_length("message") BETWEEN 20 AND 5000) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ContactInquiry_locale_check'
          AND conrelid = '"ContactInquiry"'::regclass
    ) THEN
        ALTER TABLE "ContactInquiry"
            ADD CONSTRAINT "ContactInquiry_locale_check"
            CHECK ("locale" IN ('en', 'th')) NOT VALID;
    END IF;
END
$$;
