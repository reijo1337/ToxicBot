# ToxicBot

A Go-based Telegram bot that trolls users in group chats. Combines text generation via list-based selection and LLM (DeepSeek), reacts to messages, stickers, voice messages, and chat events.

## Tech Stack

- **Language**: Go
- **Telegram framework**: `gopkg.in/telebot.v3`
- **Database**: SQLite (`jmoiron/sqlx`, migrations via `golang-migrate/migrate/v4`)
- **Configuration**: `kelseyhightower/envconfig`
- **AI**: DeepSeek API, GigaChat API (оба клиента wired-up в `cmd/main.go`)
- **Data sources**: Google Sheets (messages, stickers, voice messages, nicknames)
- **Logging**: `sirupsen/logrus`

## Project Structure

```text
cmd/main.go                              — entry point, dependency injection
db/migrations/                           — SQLite SQL migrations (5 пар up/down)
internal/
  config/config.go                       — env-based configuration (общие настройки бота)
  domain/chat/settings.go                — ChatSettings domain model
  features/
    chathistory/                         — буфер истории чата (in-memory + lazy persist в SQLite-таблицу `chat_history`)
    chatsettings/provider.go             — chat settings provider с кешем (1 min TTL)
    message/                             — message generation engine (list-based + LLM, sanitize, history prompt)
    phrase_filter/                       — meaningfulness filter for AI
    stats/                               — statistics tracking with AES encryption
  handlers/
    contract.go                          — handler interfaces (для mockgen)
    handlers.go                          — handler dispatcher (parallel execution)
    bulling/                             — main trolling handler (text responses)
    on_sticker/                          — sticker reaction handler
    on_voice/                            — voice message reaction handler
    on_user_join/                        — new member greeting handler
    on_user_left/                        — member leave reaction handler
    on_photo/                            — photo reaction handler
    personal/                            — per-user reactions (Igor, Max, Kirill)
    tagger/                              — periodic random user tagger
    settings/                            — /settings command
    stat/                                — /stat command
  infrastructure/
    ai/deepseek/                         — DeepSeek LLM client
    ai/gigachat/                         — GigaChat LLM client
    sheets/google_spreadsheet/           — Google Sheets data source
    storage/db/                          — storage layer (SQLite)
  usecase/                               — business logic
pkg/                                     — shared utilities (logger, migrator, mapper)
deploy/                                  — ansible-плейбуки деплоя (Docker): бот, Jaeger/tracing, Dozzle/логи
```

## Handlers and Features

### Bulling (`internal/handlers/bulling/`)

Main trolling mechanism. Tracks user message count via a circular list. Triggers when a user sends `threshold_count` messages within the `threshold_time` window. Also triggers on bot mentions and replies to bot messages. Enforces a `cooldown` period between responses to the same user.

### Sticker Reactions (`internal/handlers/on_sticker/`)

Reacts to stickers with probability `sticker_chance`. Replies with a random sticker from the pool (Google Sheets + Telegram sticker packs).

### Voice Reactions (`internal/handlers/on_voice/`)

Reacts to voice messages with probability `voice_chance`. Sends a voice message from Google Sheets with a simulated typing delay (0-15 seconds).

### Photo Reactions (`internal/handlers/on_photo/`)

Reacts to photos with probability `photo_chance`.

### User Join/Leave

- **Join** (`on_user_join/`): sends a greeting from Google Sheets.
- **Left** (`on_user_left/`): replies with a fixed farewell message.

### Personal (`internal/handlers/personal/`)

User-specific reactions for particular users (Igor — 1/750, Max — 1/200, Kirill — 1/150). User IDs are set via environment variables.

### Tagger (`internal/handlers/tagger/`)

Periodically tags a random chat member with an insult. Uses a priority queue (min-heap) for scheduling. Interval is randomized between `TAGGER_INTERVAL_FROM` and `TAGGER_INTERVAL_TO`.

### Settings (`internal/handlers/settings/`)

`/settings` command — admin-only in group chats:

- `/settings` — view current settings
- `/settings <key> <value>` — modify a setting
- `/settings reset` — reset to defaults

### Stats (`internal/handlers/stat/`)

`/stat` or `/stat YYYY-MM-DD`. Displays interaction statistics formatted with Telegram entities.

## Message Generation

Live в `internal/features/message/`. Две стратегии:

1. **List-Based** — random message from Google Sheets.
2. **AI** — system prompt с токсичными оскорблениями (1–2 предложения). Контролируется per-chat `ai_chance`. При ошибке LLM — fallback на list-based.

LLM-клиенты подключаются в `cmd/main.go` (DeepSeek + GigaChat). Клиент DeepSeek построен на официальном SDK `github.com/openai/openai-go/v3` через переопределённый `BaseURL`. У генератора два метода:

- `GetMessageText(replyTo, aiChance)` — одиночная реплика без контекста.
- `GetMessageTextWithHistory(history, aiChance)` — генерация с учётом истории чата из `chathistory.Buffer` (in-memory кеш размером 100 сообщений, lazy-load из SQLite-таблицы `chat_history` при первом обращении к chatID; см. `cmd/main.go:118`).

Ответ LLM прогоняется через `sanitize.go` (фильтрация артефактов) и `phrase_filter` (проверка на осмысленность).

## Configuration

Каждая подсистема парсит свои env-переменные через `envconfig` отдельно — переменные не сосредоточены в одном месте. Колонка **Source** показывает, где переменная читается.

### Required Environment Variables

| Variable | Source | Description |
|---|---|---|
| `TELEGRAM_TOKEN` | `internal/config/config.go` | Bot token from BotFather |
| `SQLITE_FILE_PATH` | `internal/config/config.go` | Path to SQLite database file |
| `DEEPSEEK_API_KEY` | `internal/infrastructure/ai/deepseek/config.go` | DeepSeek API key |
| `GIGACHAT_AUTH_KEY` | `internal/infrastructure/ai/gigachat/config.go` | GigaChat API key |
| `GOOGLE_CREDENTIALS` | `internal/infrastructure/sheets/google_spreadsheet/config.go` | JSON with Google API credentials |
| `GOOGLE_SPREADSHEET_ID` | `internal/infrastructure/sheets/google_spreadsheet/config.go` | Google Sheets spreadsheet ID |
| `IGOR_ID` / `MAX_ID` / `KIRILL_ID` | `internal/handlers/personal/personal.go` (`os.Getenv`) | Telegram user IDs для personal-хендлеров |

### Behavior and Timing

| Variable | Default | Description |
|---|---|---|
| `BULLINGS_THRESHOLD_COUNT` | 5 | Message count threshold to trigger |
| `BULLINGS_THRESHOLD_TIME` | 1m | Time window for message counting |
| `BULLINGS_COOLDOWN` | 1h | Cooldown between responses |
| `BULLINGS_AI_CHANCE` | 0.75 | Probability of AI generation |
| `STICKER_REACTIONS_CHANCE` | 0.4 | Probability of sticker reaction |
| `VOICE_REACTIONS_CHANCE` | 0.8 | Probability of voice reaction |
| `PHOTO_REACTIONS_CHANCE` | 0.75 | Probability of photo reaction |
| `STICKER_SETS` | `static_bulling_by_stickersthiefbot` | Sticker packs (comma-separated) |
| `TAGGER_INTERVAL_FROM` | 10h | Min tagger interval |
| `TAGGER_INTERVAL_TO` | 24h | Max tagger interval |
| `TELEGRAM_LONG_POLL_TIMEOUT` | 10s | Long polling timeout |

### Data Refresh Periods

| Variable | Default |
|---|---|
| `BULLINGS_UPDATE_MESSAGES_PERIOD` | 10m |
| `STICKERS_UPDATE_PERIOD` | 30m |
| `ON_USER_JOIN_UPDATE_MESSAGES_PERIOD` | 10m |
| `VOICE_UPDATE_PERIOD` | 30m |
| `NICKNAMES_UPDATE_PERIOD` | 10m |
| `GOOGLE_CACHE_INTERVAL` | 15m |

### LLM Client Tuning (опциональные)

| Variable | Default | Description |
|---|---|---|
| `DEEPSEEK_BASE_URL` | `https://api.deepseek.com/v1` | DeepSeek endpoint override (SDK appends `/chat/completions`) |
| `DEEPSEEK_TIMEOUT` | 30s | DeepSeek request timeout |
| `DEEPSEEK_MAX_RETRIES` | 3 | DeepSeek retry budget |
| `DEEPSEEK_MAX_TOKENS` | 500 | Hard cap на длину ответа в токенах. Запас над целью «300 рун»: BPE-токенайзер DeepSeek для кириллицы — примерно 1-2 символа на токен, плюс модель часто эмитирует обёртку `<msg ...></msg>` поверх полезного контента. При `finish_reason: "length"` deepseek-клиент возвращает `ErrResponseTruncated`, бот фолбэчится на list-based — слишком тесный лимит шуршит логами warning'ами и режет AI-ветку. |
| `DEEPSEEK_TEMPERATURE` | 1.1 | Sampling temperature (0-2). Между DeepSeek-дефолтом 1.0 и рекомендацией 1.3 для general chat — чуть собраннее, чтобы модель следовала правилу длины |
| `GIGACHAT_SCOPE` | `GIGACHAT_API_PERS` | GigaChat OAuth scope |
| `GIGACHAT_MODEL` | `GigaChat-Pro` | GigaChat model name |
| `GIGACHAT_TIMEOUT` | 60s | GigaChat request timeout |

### Tracing (опциональные)

Источник — `pkg/tracing/config.go`. OpenTelemetry → OTLP → Jaeger. Когда `TRACING_ENABLED=false` (по умолчанию), стоит глобальный no-op tracer и поведение не меняется.

| Variable | Default | Description |
|---|---|---|
| `TRACING_ENABLED` | `false` | Включить трейсинг (иначе no-op, ноль накладных) |
| `TRACING_OTLP_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint. В проде бот шлёт на `jaeger:4317` (DNS-алиас Jaeger в общей docker-сети `toxicbot-tracing`); локально — `localhost:4317` |
| `TRACING_SAMPLE_RATIO` | `1.0` | Доля сэмплируемых трейсов (ParentBased + TraceIDRatioBased) |
| `TRACING_CAPTURE_CONTENT` | `true` | Захват текста промптов/ответов в спанах (`false` — только длины) |
| `TRACING_SERVICE_NAME` | `toxicbot` | Имя сервиса в Jaeger |

Инфра — отдельный ручной workflow **Deploy tracing** (`workflow_dispatch`) → плейбук `deploy/deploy-tracing.yaml`: поднимает Jaeger (`jaegertracing/all-in-one`, Badger TTL `168h` = 7 дней, данные в `/srv/ToxicBot/jaeger`) и Caddy как reverse-proxy для UI с basic-auth (порт `:16686`, секреты `JAEGER_USERNAME` / `JAEGER_PASSWORD_HASH`). Jaeger и Caddy живут на docker-сети `toxicbot-tracing`; бот подключён к той же сети и шлёт OTLP на Jaeger по DNS-алиасу `jaeger:4317` (порт на хост не публикуется). Bot-плейбук сам идемпотентно создаёт сеть, поэтому порядок деплоя не важен. Деплоится независимо от релиз-цикла бота; включение трейсинга на стороне бота — repo-variable `TRACING_ENABLED` (default `true`), читается на каждом деплое. Инструментованы: root-спан на апдейт в диспатчере (`internal/handlers/handlers.go`), дочерние спаны по хендлерам (имя из `Slug()`), `gen_ai`-спаны в deepseek/gigachat, `decision`/`sanitize` в `Generator`; теггер-тег — отдельный root с `trigger=timer`.

## Per-Chat Settings

Stored in the `chat_settings` SQLite table. Cached by `chatsettings.Provider` with a 1-minute TTL. Nullable fields — global defaults are used when not overridden.

| Parameter | Type | Default |
|---|---|---|
| `threshold_count` | int | 5 |
| `threshold_time` | duration | 1m |
| `cooldown` | duration | 1h |
| `sticker_chance` | float 0.0-1.0 | 0.4 |
| `voice_chance` | float 0.0-1.0 | 0.8 |
| `ai_chance` | float 0.0-1.0 | 0.75 |
| `photo_chance` | float 0.0-1.0 | 0.75 |

## Statistics and Analytics

`response_log` table — logs every interaction. Chat ID and User ID are AES-encrypted (key passed via `-ldflags` at build time). Operation types: `on_text`, `on_sticker`, `on_voice`, `on_photo`, `on_user_join`, `on_user_left`, `personal`, `tagger`.

## Building and Running

```bash
go build -ldflags="-X main.AesKeyString=<BASE64_AES_KEY>" -o bot ./cmd/
```

AES key: 16, 24, or 32 bytes, Base64-encoded (raw, no padding).

Migrations run automatically on startup via `migrator.MigrateDB()`.

### Make targets

| Command | Purpose |
|---|---|
| `make lint` | golangci-lint v2.11.3 в Docker (конфиг `.golangci.yml`) |
| `make fmt` | Форматирование через golangci-lint |
| `make migration name=<slug>` | Создать пустую пару up/down-миграций в `db/migrations/` |
| `make align` | Авто-выравнивание полей структур (`fieldalignment -fix`) |
| `make release-patch` / `release-minor` / `release-major` | Создать и запушить новый семвер-тег (триггерит pipeline `Труба`). Только с master, чистого working tree, синхронизированного с origin. Спрашивает `[y/N]` перед push'ем. |

Тесты: `go test ./...` (отдельной make-цели нет).

## Architectural Principles

- **Interfaces** are declared in the consumer package, not the provider
- **`contract.go`** — file containing handler interfaces (for mockgen)
- **Parallel dispatch** — all handlers for the same event run in goroutines
- **Background refresh** — Google Sheets data is periodically refreshed in the background
- **Thread safety** — `sync.RWMutex` for message collections
- **Async statistics** — all `stats.Inc()` calls run asynchronously

## Gotchas

- `CLAUDE.md` — симлинк на `AGENTS.md`. Редактировать нужно `AGENTS.md`; не перезаписывать `CLAUDE.md` как обычный файл.
- При ошибке LLM (DeepSeek / GigaChat) генератор сообщений автоматически падает на list-based стратегию (Google Sheets) — отсутствие LLM-ключей не ломает бота, просто отключает AI-ветку.
- `chathistory.Buffer` загружает историю из SQLite **только при первом обращении к chatID после старта** (`ensureLoadedLocked`); до этого момента `data[chatID]` пуст. Не путать с потерей данных.
