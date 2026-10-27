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
db/migrations/                           — SQLite SQL migrations (6 пар up/down)
internal/
  config/config.go                       — env-based configuration (общие настройки бота)
  domain/chat/settings.go                — ChatSettings domain model
  features/
    chathistory/                         — буфер истории чата (in-memory + lazy persist в SQLite-таблицу `chat_history`)
    chatsettings/provider.go             — chat settings provider с кешем (1 min TTL)
    message/                             — message generation engine (list-based + LLM, sanitize, history prompt)
    phrase_filter/                       — meaningfulness filter for AI
    stats/                               — statistics tracking with AES encryption
    taggerusers/                         — реестр кандидатов теггера (память + синхронная запись в SQLite-таблицу `tagger_users`)
  handlers/
    contract.go                          — handler interfaces (для mockgen)
    handlers.go                          — handler dispatcher (parallel execution)
    bulling/                             — main trolling handler (text responses)
    on_sticker/                          — sticker reaction handler
    on_voice/                            — voice message reaction handler
    on_user_join/                        — new member greeting handler
    on_user_left/                        — member leave reaction handler
    on_photo/                            — photo reaction handler
    on_reaction/                         — персональные изменения реакций Telegram в историю, без ответа
    personal/                            — per-user reactions (Igor, Max, Kirill)
    tagger/                              — periodic random user tagger
    settings/                            — /settings command
    stat/                                — /stat command
  infrastructure/
    telegram/                            — единственный polling-поток обычных апдейтов и message_reaction
    ai/deepseek/                         — DeepSeek LLM client
    ai/gigachat/                         — GigaChat LLM client
    sheets/google_spreadsheet/           — Google Sheets data source
    storage/db/                          — storage layer (SQLite)
  usecase/                               — business logic
pkg/                                     — shared utilities (logger, migrator, mapper)
openspec/                                — living specs по capability + активные changes (см. SDD.md)
tools/specctl/                           — гейт: связывает спеки с кодом и тестами
tools/promptlab/                         — offline-стенд: варианты промпта × модели на кейсах из прода (см. README там)
deploy/                                  — ansible-плейбуки деплоя (Docker): бот, Jaeger/tracing, Dozzle/логи, лимиты хоста (журнал, swap)
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

### Message Reaction Events (`internal/handlers/on_reaction/`)

Получает новые персональные `message_reaction` через общий poller в
`internal/infrastructure/telegram`. Для доставки бот должен быть администратором чата;
тип обновления запрашивается явно. События `actor_chat`, анонимные счётчики, боты и
апдейты без достоверного пользователя или цели пропускаются. Добавления и снятия
определяются разностью наборов; замена — одно событие. Поддерживаются emoji,
custom emoji по ID и paid; неизвестные типы пропускаются.

Используется telebot v3.3.8 со штатными типами реакций. Его HTTP-клиент подключается вместе
с poller: только `getUpdates` дополнительно привязан к контексту polling, чтобы обходить
потерю сигнала stopClient в SDK при остановке. Не заменять эту пару одним штатным LongPoller:
нужны подтверждение записи до продвижения offset и прерываемые повторные ожидания.

Событие занимает одно место в общем окне 100 записей истории и сохраняется в прежнем
gob blob SQLite без миграции. Повторный update ID не дублируется, пока есть в окне,
в том числе после рестарта. Ошибка первоначального чтения истории задерживает
приём этого апдейта и остатка пачки максимум на три попытки с паузами 1 и 2 секунды.
Затем событие пропускается с Error (причина, ID апдейта и чата), offset продвигается,
приём остальных апдейтов продолжается; непрочитанный blob не перезаписывается.
Прошлые реакции и неизвестные сообщения из Telegram не восстанавливаются.

Реакция не вызывает ответ, LLM, cooldown, статистику или регистрацию участника теггера.
Её автор хранится отдельно от `SenderID` написанной реплики и не получает допуска к пингу
за одну реакцию. В промптах события отделены от сообщений, у пинга — блок `chat_reactions`;
реакция не подменяет текущую реплику и не переносит слова цели на автора действия.

Перед первым развёртыванием нужна совместимая резервная копия SQLite. Для отката старой
версии требуется восстановить копию до появления реакций: старый gob-декодер оставит
вместо событий пустые реплики. Восстановление теряет записи после копии. Процедура
проверки и фиксация результата — в `deploy/message-reactions.md` (сохраняется после архивации change).

### User Join/Leave

- **Join** (`on_user_join/`): sends a greeting from Google Sheets.
- **Left** (`on_user_left/`): replies with a fixed farewell message.

### Personal (`internal/handlers/personal/`)

User-specific reactions for particular users (Igor — 1/750, Max — 1/200, Kirill — 1/150). User IDs are set via environment variables.

### Tagger (`internal/handlers/tagger/`)

Periodically tags a random chat member with an insult. Uses a priority queue (min-heap) for scheduling, one job per non-empty chat. Interval is randomized between `TAGGER_INTERVAL_FROM` and `TAGGER_INTERVAL_TO`.

Для каждой попытки адресат выбирается из известных участников с собственными записями в используемом снимке истории по положительному `SenderID`; сообщения бота и неизвестные отправители не дают допуска. Каждый уникальный автор имеет один шанс независимо от числа сообщений. Этот же снимок передаётся генератору. Если подходящих авторов нет, пинг пропускается и перепланируется с прежним интервалом; реестр сохраняется. При старой истории без `SenderID` пинги ждут новых записей с достоверным авторством.

Кандидаты (ID + подпись) копятся по сообщениям и событиям вступления/выхода, боты исключаются. Список держит реестр `internal/features/taggerusers`: он синхронно пишет изменения в SQLite-таблицу `tagger_users` до обновления памяти, а теггер получает его зависимостью через интерфейс. При старте весь список поднимается до создания теггера (ошибка чтения валит старт), каждому непустому чату назначается новый случайный интервал; время следующего пинга не сохраняется. Перед пингом выбранный кандидат проверяется через `getChatMember`: `left`/`kicked`/`restricted` без `is_member` удаляют запись, ошибка или неоднозначный ответ лишь пропускают пинг. Ответ `getChatMember` не применяется, если за время запроса пришло противоположное изменение (выход после проверки присутствия, подтверждение присутствия после проверки отсутствия); перед отправкой проверяется, что задача чата не сменилась, а кандидат остался в списке. При вступлении нескольких пользователей одним сообщением учитываются все из `UsersJoined`. Удаление бота из чата стирает весь чат. `/settings tagger off` глушит пинги чата: попытка пропускается до `getChatMember` и перепланируется, а сбор участников продолжается; ошибка чтения настроек теггер не выключает.

### Settings (`internal/handlers/settings/`)

`/settings` command — все формы доступны только владельцу и администраторам текущей группы
или супергруппы (в личке отказывает):

- `/settings` — view current settings
- `/settings <key> <value>` — modify a setting
- `/settings reset` — reset to defaults

Перед каждой командой роль отправителя заново проверяется через Telegram `getChatMember`.
Анонимный администратор может вызвать команду от имени самой текущей супергруппы;
сообщения от имени другого чата или канала не предоставляют доступ. При невозможности
подтвердить права команда не выполняется. Для гарантированной проверки других пользователей
сам бот должен быть администратором чата.

### Stats (`internal/handlers/stat/`)

`/stat` or `/stat YYYY-MM-DD`. Displays interaction statistics formatted with Telegram entities.

## Message Generation

Live в `internal/features/message/`. Две стратегии:

1. **List-Based** — random message from Google Sheets.
2. **AI** — system prompt с токсичными оскорблениями (1–2 предложения). Контролируется per-chat `ai_chance`. При ошибке LLM — fallback на list-based.

LLM-клиенты подключаются в `cmd/main.go` (DeepSeek + GigaChat). Клиент DeepSeek построен на официальном SDK `github.com/openai/openai-go/v3` через переопределённый `BaseURL`. У генератора два метода:

- `GetMessageText(replyTo, aiChance)` — одиночная реплика без контекста.
- `GetMessageTextWithHistory(history, aiChance)` — генерация с учётом истории чата из `chathistory.Buffer` (in-memory кеш размером 100 записей, включая события реакций, lazy-load из SQLite-таблицы `chat_history` при первом обращении к chatID).

Ответ LLM прогоняется через `sanitize.go` (фильтрация артефактов) и `phrase_filter` (проверка на осмысленность).

### Контекст ссылок

`internal/features/linkcontext` обогащает первую HTTP(S)-ссылку из каждого текстового сообщения
(включая пропуск реакции) и из подписи обрабатываемого фото. Скрытые Telegram-ссылки читаются
по UTF-16-смещениям; их видимая подпись сохраняется как `anchor`. Остальные ссылки не загружаются,
даже если первая запрещена, недоступна или не содержит OG. Настройки карточки Telegram не влияют.
Фото, пропущенные по прежним правилам, не обогащаются и не записываются.

Из HTML head берутся первые непустые `og:title`, `og:description` и `og:image`. Превью JPEG/PNG
описывается существующим GigaChat с отдельным промптом; сбой превью сохраняет текстовые поля.
Результат — поле-значение `chathistory.Entry.Link`, сохраняемое в прежнем gob blob SQLite без
миграции. Старые записи читаются без повторных запросов. Ответы, фото и пинги получают отдельный
санированный `<link>` при исходной реплике: URL, anchor, заголовок, описание и машинный пересказ
превью не становятся словами или личными фактами отправителя. Лимиты полей: URL 512, anchor и
заголовок 200, описания 500 рун; осмысленность проверяется по обычному тексту до 1000 байт UTF-8.

Один общий экземпляр подключён в `cmd/main.go`. Кеш LRU на 512 URL сохраняет query, нормализует
схему/hostname/стандартный порт и убирает fragment. TTL: успех 1 час, нет OG или сбой превью
5 минут, ошибка страницы 1 минута. Anchor и исходное написание URL принадлежат сообщению и
не кешируются. Одновременные обращения объединяются; отмена одного ожидания не отменяет другие.
Бюджет сообщения и общей работы — по 20 секунд, запроса — 3 секунды, vision — 12 секунд.
Не более 8 новых работ без очереди, 4 HTTP-загрузок и 2 vision-вызовов одновременно.
Частоты: чат 6/мин (burst 2), HTTP процесса 60/мин и hostname 12/мин (burst 8), vision 10/мин
(burst 2). Таблицы лимитеров ограничены 512 записями, неактивные удаляются через 10 минут.
Обычные фото используют тот же лимит vision: при занятых слотах или исчерпанной квоте работают
прежние правила ошибки описания, очередь фотографий не создаётся. Их промпт и таймаут не меняются.
Решение текстовой реакции, cooldown и снимок истории фиксируются до ожидания обогащения,
чтобы более поздняя реплика не стала целью текущего ответа. Ожидания OAuth и retry backoff
GigaChat прерываются по контексту.

`internal/infrastructure/linkpreview` использует отдельный анонимный HTTP/1.1 transport без proxy,
cookies, Referer, авторизации, trace-заголовков, сжатия, keep-alive и автоматических повторов.
Разрешены только публичные адреса и стандартные порты, проверяются все DNS-ответы и каждый
редирект, соединение закрепляется за проверенным IP, TLS проверяется, HTTPS downgrade запрещён.
На цепочку до 3 редиректов; HTML до 1 MiB, картинка до 5 MiB и 20 млн пикселей, заголовки до
32 KiB. HTML и выход моделей не запускают другие запросы. Новая диагностика не содержит полных
URL, path/query, Location, метаданных или необработанных сетевых ошибок.

Публичный GET всё ещё может вызвать побочный эффект на стороне сайта и раскрывает IP сервера
бота. Исходная реплика и обогащение остаются в существующем контуре SQLite и LLM; при
`TRACING_CAPTURE_CONTENT=true` промпт также попадает в Jaeger. Значение `false` отключает текст
в трейсах, но не передачу контекста провайдеру LLM. Перед релизом проверять toolchain бинарника
через `go version -m` и достижимые уязвимости HTTP/HTML/image-пути через `govulncheck`.
Минимальная версия Go — 1.26.10; binary/lint workflows берут её из `go.mod`.

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
| `PHOTO_REACTIONS_CHANCE` | 1 | Probability of photo reaction |
| `STICKER_SETS` | `static_bulling_by_stickersthiefbot` | Sticker packs (comma-separated) |
| `TAGGER_INTERVAL_FROM` | 10h | Min tagger interval |
| `TAGGER_INTERVAL_TO` | 24h | Max tagger interval |
| `TAGGER_ENABLED` | true | Глобальное умолчание per-chat настройки `tagger` |
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
| `DEEPSEEK_MODEL` | `deepseek-flash` | Id модели. `deepseek-flash` — V4.1-Flash; старые `deepseek-v4-flash` и `deepseek-v4-pro` сняты 2026-09 и роутятся сюда же |
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
| `photo_chance` | float 0.0-1.0 | 1 |
| `tagger` (колонка `tagger_enabled`) | on/off | on (`TAGGER_ENABLED`) |

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
| `make spec-new name=<id>` | Создать change в `openspec/changes/` |
| `make spec-check` | Гейт: структура спек, связь с кодом и тестами, храповик покрытия |
| `make spec-merge-check` | Гейт перед merge: нет активных changes |
| `make spec-status` | Покрытие по capability, активные changes, технический долг |
| `make spec-cover cap=<имя>` | Сценарии одной capability со статусами |
| `make spec-archive name=<id>` | Влить дельту в living specs и заархивировать change |
| `make test` | `go test ./...` |
| `make promptlab-fetch host=<ip>` / `promptlab-extract` / `promptlab-run` | Стенд сравнения промптов и моделей на реальных кейсах из прод-базы; данные в `tools/promptlab/data/` (gitignore). Ключи — `DEEPSEEK_API_KEY`, `OPENROUTER_API_KEY` |
| `make release-patch` / `release-minor` / `release-major` | Создать и запушить новый семвер-тег (триггерит pipeline `Труба`). Только с master, чистого working tree, синхронизированного с origin. Спрашивает `[y/N]` перед push'ем. |

Тесты: `make test` или `go test ./...`.

## Architectural Principles

- **Interfaces** are declared in the consumer package, not the provider
- **`contract.go`** — file containing handler interfaces (for mockgen)
- **Parallel dispatch** — all handlers for the same event run in goroutines
- **Background refresh** — Google Sheets data is periodically refreshed in the background
- **Thread safety** — `sync.RWMutex` for message collections
- **Async statistics** — all `stats.Inc()` calls run asynchronously

## Спеки и изменения

Репозиторий на уровне spec-anchored: наблюдаемое поведение бота описано в `openspec/specs/`, а
каждое его изменение проходит через change в `openspec/changes/`. Полное руководство —
**`SDD.md`** в корне, здесь только правила.

**Изменение наблюдаемого поведения начинается с дельты, а не с кода.** Если по ходу работы решение
поменялось — сначала правится change, потом код.

Маршрут по риску:

| Режим | Когда | Артефакты |
|---|---|---|
| Без спеки | опечатка, рефакторинг без смены поведения, зависимости, деплой, CI | PR-описание |
| Lite | понятный баг, локальная правка хендлера, другой текст | `proposal.md` + `tasks.md`, дельта — если меняется ожидаемое поведение |
| Полный | промпты и `sanitize`; вероятности и дефолты; миграции; формат `chat_settings`, `response_log`, `chat_history`, `tagger_users`; смена модели или её параметров; новый хендлер | proposal + дельта + `design.md` + `tasks.md` |

Появился новый контракт (схема БД, формат промпта, внешний API) — режим поднимается до полного.

Правила:

- Сценарий получает идентификатор `[PREFIX-NNN]`, тест — якорь `// spec: PREFIX-NNN` над функцией.
- Ссылок на идентификаторы сценариев в рабочем коде быть не должно — только в тестах.
- Спека описывает наблюдаемое поведение; имена типов, функций и структура пакетов туда не попадают.
- Новый или изменённый сценарий обязан быть `covered` либо `manual` с причиной.
- Каталог `openspec/changes/<id>/` со всеми артефактами сохраняется в ПР на протяжении ревью.
  Подготовка и публикация ПР не включают архивацию, даже если реализация и тесты уже завершены.
- `make spec-archive` выполняется **после завершения ревью, непосредственно перед мержем, в той же
  ветке**; он вливает дельту и удаляет каталог change. Задача архивации остаётся открытой до этого
  этапа. После архивации повторяются проверки, изменения публикуются в ПР, перед мержем нужно
  дождаться успешного CI. Архив завершённых предложений в репозитории не хранится: мотивация
  остаётся в истории коммитов, а долгоживущие решения уезжают ADR'ом в вику.
- При ревью проверяется spec drift: изменение поведения без change, дельта не совпадает с кодом.
  Обязательная CI-проверка `changes-archived` запрещает merge в `master`, пока есть активные changes,
  включая Lite без дельты. Во время ревью она остаётся красной до архивации; локально — `make spec-merge-check`.
- Обход гейта — строка `[skip-spec: причина]` отдельной строкой в теле коммита; причина обязательна.

### Ветки и коммиты

**Одно полноценное изменение — одна ветка и один коммит.** Промежуточные шаги внутри работы не
попадают в историю: перед PR ветка сквошится в один коммит с осмысленным сообщением. Заголовок —
короткий, в нижнем регистре, как в существующей истории; тело объясняет, что изменилось и почему,
а не перечисляет шаги.
Архивация после ревью включается в тот же итоговый коммит перед мержем.

## Gotchas

- Рабочий стейт агентных сессий (`docs/superpowers/`, `.superpowers/`) в git не попадает. Контракт изменения живёт в `openspec/changes/`, черновики и планы по дороге к нему — нет.
- `CLAUDE.md` — симлинк на `AGENTS.md`. Редактировать нужно `AGENTS.md`; не перезаписывать `CLAUDE.md` как обычный файл.
- При ошибке LLM (DeepSeek / GigaChat) генератор сообщений автоматически падает на list-based стратегию (Google Sheets) — отсутствие LLM-ключей не ломает бота, просто отключает AI-ветку.
- `chathistory.Buffer` загружает историю из SQLite **только при первом обращении к chatID после старта** (`ensureLoadedLocked`); до этого момента `data[chatID]` пуст. Не путать с потерей данных.
