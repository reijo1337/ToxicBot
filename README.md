# ToxicBot

Telegram-бот, который будет вас булить в вашем собственном чате.

[![Труба Actions Status](https://github.com/reijo1337/ToxicBot/workflows/Труба/badge.svg)](https://github.com/reijo1337/ToxicBot/actions) [![Lint Actions Status](https://github.com/reijo1337/ToxicBot/workflows/lint/badge.svg)](https://github.com/reijo1337/ToxicBot/actions)

## Env

* `BULLINGS_COOLDOWN` - время, которое бот выжидает после того, как набросит на вентилятор пользователю. По умолчанию `1h`.
* `BULLINGS_AI_CHANCE` - вероятность отправки сообщения из марковской цепи. По умолчанию `0.75`.
* `BULLINGS_THRESHOLD_COUNT` - порог отправленных подряд сообщений, после которых бот вступает в игру. По умолчанию `5`.
* `BULLINGS_THRESHOLD_TIME` - время, за которое пользователь должен отправить `BULLINGS_THRESHOLD_COUNT` сообщений, чтобы бот обратил на него внимание. По умолчанию `1m`.
* `BULLINGS_UPDATE_MESSAGES_PERIOD` - Интервал обновления сообщений из дока. По умолчанию `10m`
* `GOOGLE_CACHE_INTERVAL` - На сколько клиент гугла будет кешировать ответ
* `GOOGLE_CREDENTIALS` - Json с доступами до гугла 
* `GOOGLE_SPREADSHEET_ID` - ID таблицы в google spreadsheet
* `IGOR_ID` - ???
* `ON_USER_JOIN_UPDATE_MESSAGES_PERIOD` - Интервал обновления сообщений из дока. По умолчанию `10m`
* `STICKER_REACTIONS_CHANCE` - вероятность реакции на отправленный стикер. По умолчанию `0.4`
* `STICKER_SETS` - Список стикер паков через запятую. По умолчанию `static_bulling_by_stickersthiefbot`
* `STICKERS_UPDATE_PERIOD` - Интервал обновления стикеров из дока. По умолчанию `30m`
* `TELEGRAM_TOKEN` - токен telegram-бота. Обязателен.
* `VOICE_REACTIONS_CHANCE` - вероятность реакции на отправленный войс. По умолчанию `0.4`
* `VOICE_UPDATE_PERIOD` - Интервал обновления сообщений из дока. По умолчанию `30m`
* `DEEPSEEK_API_KEY` - ключ апи дипсика
* `GIGACHAT_AUTH_KEY` - ключ апи ГигаЧат для распознавания картинок

## Tracing

Трейсинг (OpenTelemetry → Jaeger) показывает, как сформировался каждый ответ бота: трейсы по хендлерам, разворачиваются в спаны (`decision` → `gen_ai` → `sanitize` → send) с input/output LLM.

**Прод:** Jaeger разворачивается отдельным ручным GitHub Action **Deploy tracing** (`workflow_dispatch`) → плейбук `deploy/deploy-tracing.yaml`. Он поднимает Jaeger и Caddy (reverse-proxy для UI с basic-auth, порт `:16686`, секреты `JAEGER_USERNAME` / `JAEGER_PASSWORD_HASH`). Бот и Jaeger живут в общей docker-сети `toxicbot-tracing`; бот шлёт трейсы на `jaeger:4317` (по DNS-алиасу внутри сети, OTLP-порт на хост не публикуется). Включение — repo-variable `TRACING_ENABLED` (default `true`), подхватывается при деплое бота.

**Локально:** подними Jaeger одним контейнером и запусти бота с трейсингом в его сторону:

```sh
docker run -d --name jaeger -p 4317:4317 -p 16686:16686 \
  -e COLLECTOR_OTLP_ENABLED=true jaegertracing/all-in-one:1.57
TRACING_ENABLED=true TRACING_OTLP_ENDPOINT=localhost:4317 ./bot
```

UI — <http://localhost:16686>, сервис `toxicbot`, фильтр по полю Operation (имя хендлера).

* `TRACING_ENABLED` - включить трейсинг. По умолчанию `false` (no-op, ноль накладных расходов).
* `TRACING_OTLP_ENDPOINT` - OTLP gRPC-эндпоинт. По умолчанию `localhost:4317` (в проде — `jaeger:4317`, по DNS-алиасу в сети `toxicbot-tracing`).
* `TRACING_SAMPLE_RATIO` - доля сэмплируемых трейсов (0.0–1.0). По умолчанию `1.0`.
* `TRACING_CAPTURE_CONTENT` - захватывать ли текст промптов/ответов в спанах. По умолчанию `true`; при `false` пишутся только длины.
* `TRACING_SERVICE_NAME` - имя сервиса в Jaeger. По умолчанию `toxicbot`.

Ретеншн трейсов — 7 дней (`BADGER_SPAN_STORE_TTL=168h` на контейнере Jaeger; объём выходит на плато).

> ⚠️ При `TRACING_CAPTURE_CONTENT=true` (по умолчанию) сырые промпты, ответы LLM и тексты сообщений чата хранятся в Jaeger эти 7 дней. Для приватных чатов ставь `TRACING_CAPTURE_CONTENT=false` — тогда в спанах остаются только длины.
