# promptlab

Offline-стенд: прогоняет варианты системного промпта и разные модели по реальным
ситуациям из прода и складывает ответы рядом в markdown, чтобы сравнивать глазами.

Путь генерации тот же, что в бою: харнесс собирает настоящий `message.Generator`,
так что envelope `<msg from= now=>`, сортировка истории, блок `<examples>` и
`sanitize` совпадают с продом. Отличаются только промпт (если подменён) и модель.

## Данные

Каталог `data/` в gitignore: там переписка чатов и отчёты.

| Файл | Откуда |
|---|---|
| `data/sas_sqlite.db` | копия прод-базы: `make promptlab-fetch` |
| `data/cases.json` | `make promptlab-extract` |
| `data/examples.txt` | фразы из Google Sheets (вкладка с сообщениями), по одной на строку |
| `data/report.md` | результат `make promptlab-run` |

Кейс — окно истории чата, в конце которого бот в проде ответил. Триггер — последняя
реплика человека, ответ из прода лежит в колонке «В проде». Для фото-кейсов
повторяется путь `on_photo`: ответы бота выкидываются из окна, добавляется
анти-шаблонный steering.

## Промпты

`prompts/*.txt` — варианты базовой части системного промпта (всё до `<examples>`).
Имя файла — имя варианта в отчёте. `baseline` зарезервирован за промптом из кода и
гоняется всегда, если не передать `--no-baseline`.

Стартовый вариант: `go run ./tools/promptlab dump-prompt --out tools/promptlab/prompts/v2.txt`.

## Модели

`promptlab.yaml` — список OpenAI-совместимых точек входа. Ключи только через
переменные окружения. Поля `extra_body` уходят в тело запроса как есть
(`thinking: {type: disabled}` для DeepSeek).

## Запуск

Ключи — в окружении или в `data/.env` (формат `KEY=value`, каталог в gitignore):

```bash
make promptlab-fetch host=95.81.112.114   # копия базы с VDS
make promptlab-extract                    # data/cases.json
make promptlab-run                        # data/report.md, все варианты × все модели
```

Точечно:

```bash
make promptlab-run args="--models deepseek-flash --variants baseline,v2 --limit 10 --kind text"
```
