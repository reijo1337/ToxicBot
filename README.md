# ToxicBot

Telegram-бот, который будет вас булить в вашем собственном чате.

[![Труба Actions Status](https://github.com/reijo1337/ToxicBot/workflows/Труба/badge.svg)](https://github.com/reijo1337/ToxicBot/actions)

[![Lint Actions Status](https://github.com/reijo1337/ToxicBot/workflows/Lint/badge.svg)](https://github.com/reijo1337/ToxicBot/actions)

## Env

* `BULLINGS_COOLDOWN` - время, которое бот выжидает после того, как набросит на вентилятор пользователю. По умолчанию `1h`.
* `BULLINGS_MARKOV_CHANCE` - вероятность отправки сообщения из марковской цепи. По умолчанию `0.75`.
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
