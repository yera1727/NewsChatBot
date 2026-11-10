-- Источники новостей становятся отдельной сущностью.
-- Миграция переносит существующие данные: ни статьи, ни подписки не теряются.

CREATE TABLE sources (
    id         BIGSERIAL   PRIMARY KEY,
    url        TEXT        NOT NULL UNIQUE,
    title      TEXT        NOT NULL DEFAULT '',   -- название ленты из RSS, заполняет fetcher
    active     BOOLEAN     NOT NULL DEFAULT true, -- false — fetcher ленту не опрашивает
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 1. Заводим источники из уже собранных статей.
INSERT INTO sources (url)
SELECT DISTINCT source FROM articles ORDER BY source;

-- 2. articles: текстовая колонка source → ссылка source_id на sources.
ALTER TABLE articles ADD COLUMN source_id BIGINT REFERENCES sources (id);
UPDATE articles a SET source_id = s.id FROM sources s WHERE s.url = a.source;
ALTER TABLE articles ALTER COLUMN source_id SET NOT NULL;
ALTER TABLE articles DROP COLUMN source; -- индекс articles_source_id_idx (source, id) удалится вместе с колонкой
CREATE INDEX articles_by_source_idx ON articles (source_id, id);

-- 3. subscriptions: то же самое. При удалении источника его подписки удаляются (CASCADE).
ALTER TABLE subscriptions ADD COLUMN source_id BIGINT REFERENCES sources (id) ON DELETE CASCADE;
UPDATE subscriptions sub SET source_id = s.id FROM sources s WHERE s.url = sub.source;
DELETE FROM subscriptions WHERE source_id IS NULL; -- подписки на несуществующие источники
ALTER TABLE subscriptions ALTER COLUMN source_id SET NOT NULL;
ALTER TABLE subscriptions DROP CONSTRAINT subscriptions_pkey;
ALTER TABLE subscriptions DROP COLUMN source;
ALTER TABLE subscriptions ADD PRIMARY KEY (chat_id, source_id);
