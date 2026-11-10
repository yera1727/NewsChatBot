-- Откат: возвращаем текстовую колонку source и удаляем таблицу sources.

ALTER TABLE subscriptions ADD COLUMN source TEXT;
UPDATE subscriptions sub SET source = s.url FROM sources s WHERE s.id = sub.source_id;
ALTER TABLE subscriptions ALTER COLUMN source SET NOT NULL;
ALTER TABLE subscriptions DROP CONSTRAINT subscriptions_pkey;
ALTER TABLE subscriptions DROP COLUMN source_id;
ALTER TABLE subscriptions ADD PRIMARY KEY (chat_id, source);

ALTER TABLE articles ADD COLUMN source TEXT;
UPDATE articles a SET source = s.url FROM sources s WHERE s.id = a.source_id;
ALTER TABLE articles ALTER COLUMN source SET NOT NULL;
DROP INDEX IF EXISTS articles_by_source_idx;
ALTER TABLE articles DROP COLUMN source_id;
CREATE INDEX articles_source_id_idx ON articles (source, id);

DROP TABLE sources;
