-- Add Markdown article content while preserving the legacy structured sections for reads.
ALTER TABLE "ArticleTranslation"
    ADD COLUMN IF NOT EXISTS "content" TEXT NOT NULL DEFAULT '';

-- Escape legacy plain text before projecting it into Markdown. The temporary
-- helper exists only for this migration session and cannot affect runtime SQL.
CREATE OR REPLACE FUNCTION pg_temp.escape_article_markdown(input TEXT)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
STRICT
AS $$
    SELECT replace(
        replace(
            replace(
                replace(
                    replace(
                        replace(
                            replace(
                                replace(
                                    replace(
                                        replace(
                                            replace(
                                                replace(
                                                    replace(
                                                        replace(
                                                            replace(
                                                                replace(
                                                                    replace(
                                                                        replace(input, '&', '&amp;'),
                                                                        '<', '&lt;'
                                                                    ),
                                                                    '>', '&gt;'
                                                                ),
                                                                E'\\', E'\\\\'
                                                            ),
                                                            '`', E'\\`'
                                                        ),
                                                        '*', E'\\*'
                                                    ),
                                                    '_', E'\\_'
                                                ),
                                                '{', E'\\{'
                                            ),
                                            '}', E'\\}'
                                        ),
                                        '[', E'\\['
                                    ),
                                    ']', E'\\]'
                                ),
                                '(', E'\\('
                            ),
                            ')', E'\\)'
                        ),
                        '#', E'\\#'
                    ),
                    '+', E'\\+'
                ),
                '-', E'\\-'
            ),
            '!', E'\\!'
        ),
        '|', E'\\|'
    );
$$;

-- Backfill existing structured articles so content is not lost when the public
-- renderer switches from sections[] to Markdown content.
UPDATE "ArticleTranslation" AS translation
SET "content" = COALESCE(
    (
        SELECT string_agg(
            concat(
                '## ',
                pg_temp.escape_article_markdown(section.item ->> 'heading'),
                E'\n\n',
                COALESCE(
                    (
                        SELECT string_agg(
                            pg_temp.escape_article_markdown(paragraph.value),
                            E'\n\n' ORDER BY paragraph.position
                        )
                        FROM jsonb_array_elements_text(
                            COALESCE(section.item -> 'paragraphs', '[]'::jsonb)
                        ) WITH ORDINALITY AS paragraph(value, position)
                    ),
                    ''
                )
            ),
            E'\n\n' ORDER BY section.position
        )
        FROM jsonb_array_elements(COALESCE(translation."sections", '[]'::jsonb))
            WITH ORDINALITY AS section(item, position)
    ),
    ''
)
WHERE translation."content" = '';
