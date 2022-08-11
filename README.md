# ToxicBot

Telegram-бот, который будет вас булить в вашем собственном чате.

[![Труба Actions Status](https://github.com/reijo1337/ToxicBot/workflows/Труба/badge.svg)](https://github.com/reijo1337/ToxicBot/actions)

## Env

* `TELEGRAM_TOKEN` - токен telegram-бота. Обязателен.
* `BULLINGS_FILE` - путь до файла с фразами для ругани. Обязателен.
* `BULLINGS_THRESHOLD_COUNT` - порог отправленных подряд сообщений, после которых бот вступает в игру. По умолчанию `5`.
* `BULLINGS_THRESHOLD_TIME` - время, за которое пользователь должен отправить `BULLINGS_THRESHOLD_COUNT` сообщений, чтобы бот обратил на него внимание. По умолчанию `1m`.
* `BULLINGS_COOLDOWN` - время, которое бот выжидает после того, как набросит на вентилятор пользователю. По умолчанию `1h`.
* `GREETINGS_PATH` - файл с фразами, которыми бот приветствует новых членов чата. Обязателен.
* `IGOR_ID` - ???
* `IGOR_FILE_PATH` - ???
* `BULLINGS_MARKOV_CHANCE` - вероятность отправки сообщения из марковской цепи. По умолчанию `0.75`.
* `STICKERS_FILE` - путь до файла со стикерами. По умолчанию `data/stickers`
* `STICKER_REACTIONS_CHANCE` - вероятность реакции на отправленный стикер. По умолчанию `0.4`
* `STICKER_SETS` - Список стикер паков через запятую. По умолчанию `static_bulling_by_stickersthiefbot`
