# Trip Service

Сервис лабораторной работы 1. Создает, возвращает и завершает поездки.
Данные хранятся в PostgreSQL.

## Требования

- Go 1.26
- Docker
- tripgoctl

## Запуск

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect

make migrate
make run
```

Проверка сервиса:

```bash
curl -i localhost:8080/health
curl -i localhost:8080/ready
```

## Команды

```bash
make generate       # генерация Go-кода из OpenAPI
make migrate        # применение миграций
make migrate-down   # откат последней миграции
make run            # запуск сервиса
make test           # тесты с race detector
make build          # сборка проекта
make docker-build   # сборка Docker-образа
```

## Переменные окружения

| Переменная | Назначение |
|---|---|
| `HTTP_ADDR` | адрес HTTP-сервера |
| `HTTP_READ_TIMEOUT` | таймаут чтения запроса |
| `HTTP_READ_HEADER_TIMEOUT` | таймаут чтения заголовков |
| `HTTP_WRITE_TIMEOUT` | таймаут записи ответа |
| `HTTP_IDLE_TIMEOUT` | таймаут keep-alive соединения |
| `LOG_LEVEL` | уровень логирования |
| `SHUTDOWN_TIMEOUT` | таймаут graceful shutdown |
| `IDEMPOTENCY_KEY_TTL` | срок действия ключа идемпотентности |
| `DATABASE_URL` | строка подключения к PostgreSQL |
| `DATABASE_MAX_CONNS` | максимальный размер пула соединений |
| `DATABASE_MIN_CONNS` | минимальный размер пула соединений |
| `DATABASE_MAX_CONN_LIFETIME` | максимальное время жизни соединения |
| `DATABASE_CONNECT_TIMEOUT` | таймаут подключения к PostgreSQL |
| `DATABASE_QUERY_TIMEOUT` | таймаут операции с PostgreSQL |

Пример находится в `.env.example`. Локальный `.env` создает `tripgoctl`.

## HTTP API

| Метод | Путь | Назначение |
|---|---|---|
| `POST` | `/api/v1/trips` | создать поездку |
| `GET` | `/api/v1/trips/{tripId}` | получить поездку |
| `POST` | `/api/v1/trips/{tripId}/finish` | завершить поездку |
| `GET` | `/health` | проверить процесс |
| `GET` | `/ready` | проверить PostgreSQL |

Источник контракта: `contracts/openapi/trip-service.openapi.yaml`.
Сгенерированный файл: `api/api.gen.go`.

## Решения

### Транзакции

`TxManager.Do` открывает транзакцию и помещает ее в `context.Context`.
Репозиторий использует транзакцию из контекста, а при ее отсутствии работает
через пул. Вложенный `Do` переиспользует текущую транзакцию. Ошибка и panic
приводят к rollback.

Используется уровень `READ COMMITTED`. Необходимые конкурентные гарантии
обеспечиваются ограничениями и условными запросами PostgreSQL.

Создание поездки и начальная запись в `trip_status_history` выполняются в одной
транзакции.

### Одна активная поездка водителя

Частичный уникальный индекс по `driver_id` запрещает две активные поездки у
одного водителя. Ошибка PostgreSQL `23505` преобразуется в `409 driver_busy`.

### Конкурентное завершение

Завершение использует `UPDATE` с условием `status = 'active'`. Поэтому только
один конкурентный запрос изменяет поездку. Остальные получают
`409 trip_completed`.

### Идемпотентность

`POST /api/v1/trips` поддерживает заголовок `Idempotency-Key`.

- первый запрос возвращает `201`;
- повтор с тем же ключом и телом возвращает `200` и ту же поездку;
- тот же ключ с другим телом возвращает `409 idempotency_conflict`.

Хеш запроса и `trip_id` хранятся в PostgreSQL. Ключ резервируется в той же
транзакции, что и поездка. Срок действия задается через
`IDEMPOTENCY_KEY_TTL`, значение в `.env.example` равно `24h`.

## Docker

Итоговый образ основан на `scratch`, содержит только бинарник и запускается от
пользователя `65532:65532`.

```bash
make docker-build

set -a
source .env
set +a

docker run --rm \
  --add-host host.docker.internal:host-gateway \
  --env-file .env \
  -e DATABASE_URL="${DATABASE_URL/localhost/host.docker.internal}" \
  -p 8080:8080 \
  trip-service:local
```

Размер образа: 15.9 MB по выводу `docker images trip-service:local`.
