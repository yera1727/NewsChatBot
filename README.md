# NewsChatBot

Telegram-бот, который собирает новости из публичных Telegram-каналов и RSS-лент
и присылает новые посты подписчикам.

Пет-проект на Go: два микросервиса, PostgreSQL, Docker, Clean Architecture.

## Возможности

| Команда | Что делает |
|---|---|
| `/add @канал` | добавить Telegram-канал (или RSS-ленту по ссылке) и подписаться на него |
| `/sources` | список источников |
| `/subscribe N`, `/unsubscribe N` | подписаться / отписаться от источника номер N |
| `/my` | мои подписки |
| `/latest` | последние новости из моих подписок |

Новые посты по подпискам бот присылает сам.

## Архитектура

```
 Telegram-каналы (t.me/s/…)  ┐                                   ┌──→ Telegram
 RSS-ленты                   ┴─→ fetcher ──→ PostgreSQL ←── bot ─┘
                                (раз в 5 мин)              (команды + рассылка)
```

- **fetcher** — опрашивает источники параллельно (горутины) и сохраняет новые посты без дублей.
- **bot** — отвечает на команды и раз в 30 секунд рассылает подписчикам то, что они ещё не получили.
- Сервисы не вызывают друг друга напрямую — их связывает только база.

Код разложен по слоям Clean Architecture, зависимости направлены внутрь:

```
cmd/fetcher, cmd/bot     точки входа: собирают зависимости и запускают сервис
internal/domain          сущности: Source, Article (ни от чего не зависит)
internal/usecase         бизнес-логика и интерфейсы, которые ей нужны
internal/adapter/        реализации интерфейсов:
  ├── postgres           хранилище
  ├── rss                RSS-ленты
  ├── tgchannel          публичные Telegram-каналы (разбор веб-превью)
  ├── feedrouter         выбор адаптера по ссылке
  ├── safehttp           HTTP-клиент с защитой от SSRF
  └── telegram           Telegram-бот
migrations/              SQL-миграции схемы БД
```

## Запуск

Нужен Docker (на Windows — Docker Desktop).

```bash
cp .env.example .env      # и вписать TELEGRAM_TOKEN от @BotFather
docker compose up -d --build
```

Поднимутся PostgreSQL, миграции, fetcher и bot. Все сервисы перезапускаются
сами при падении и после перезагрузки (`restart: unless-stopped`).

```bash
docker compose ps                      # статус
docker compose logs -f bot fetcher     # логи
docker compose up -d --build           # обновить после изменений в коде
docker compose down                    # остановить (данные сохранятся)
```

> ⚠️ Один токен — один запущенный бот. Если бот работает в Docker, не запускайте
> его одновременно из GoLand (и наоборот): Telegram вернёт ошибку конфликта.

### Разработка в GoLand

```bash
docker compose up -d postgres migrate   # только база
```

Затем запустить `cmd/fetcher` и `cmd/bot` из GoLand. Настройки читаются из `.env`;
для запуска вне Docker в нём должен быть
`DATABASE_URL=postgres://newsbot:newsbot@localhost:5433/newsbot?sslmode=disable`.

Тесты: `go test ./...`

## Деплой на сервер (VPS)

Подойдёт минимальный VPS (1 CPU, 1 ГБ RAM) с Ubuntu в стране, где открыт `t.me`.

```bash
# 1. Установить Docker
curl -fsSL https://get.docker.com | sh

# 2. Забрать код
git clone https://github.com/yera1727/NewsChatBot.git
cd NewsChatBot
git checkout master

# 3. Настройки: токен и надёжный пароль БД
cp .env.example .env
nano .env    # TELEGRAM_TOKEN=..., POSTGRES_PASSWORD=<результат: openssl rand -hex 24>

# 4. Запуск
docker compose up -d --build
```

Обновление: `git pull && docker compose up -d --build`.

После запуска на сервере остановите локальную копию (`docker compose down` на
своём компьютере) — см. предупреждение про токен выше.

### Резервная копия базы

```bash
docker compose exec postgres pg_dump -U newsbot newsbot > backup.sql            # сохранить
docker compose exec -T postgres psql -U newsbot newsbot < backup.sql            # восстановить
```

## Ограничения

- Читаются только **публичные** Telegram-каналы (с @username).
- Каналы читаются через веб-превью `t.me/s/`, а не официальный API: если Telegram
  поменяет вёрстку, адаптер `tgchannel` нужно будет поправить.
- RSS: только формат RSS 2.0 в кодировке UTF-8.
