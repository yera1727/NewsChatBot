CREATE TABLE articles (
    id           BIGSERIAL   PRIMARY KEY,
    source       TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    link         TEXT        NOT NULL UNIQUE, -- UNIQUE не даст сохранить одну статью дважды
    description  TEXT        NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ,                 -- NULL, если лента не указала дату
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Бот будет часто запрашивать «свежие новости», индекс ускорит сортировку по дате.
CREATE INDEX articles_published_at_idx ON articles (published_at DESC);
