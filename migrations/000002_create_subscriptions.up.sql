CREATE TABLE subscriptions (
    chat_id         BIGINT      NOT NULL,           -- id чата в Telegram
    source          TEXT        NOT NULL,           -- лента (совпадает с articles.source)
    last_article_id BIGINT      NOT NULL DEFAULT 0, -- до какой статьи уже доставлено
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, source)                   -- на один источник можно подписаться один раз
);

-- Ускоряет поиск «статьи источника X новее id Y» при рассылке.
CREATE INDEX articles_source_id_idx ON articles (source, id);
