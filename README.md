# Trip Service

Сервис создает, возвращает и завершает поездки. Данные хранятся в PostgreSQL.
В ЛР2 добавлены тесты, проверка архитектуры линтером и телеметрия.

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
make generate       # генерация Go-кода из OpenAPI и моков
make migrate        # применение миграций
make migrate-down   # откат последней миграции
make run            # запуск сервиса
make test           # тесты с race detector и покрытие домена и usecase
make test-integration # тесты с реальным PostgreSQL
make lint           # golangci-lint с проверкой зависимостей между слоями
make build          # сборка проекта
make docker-build   # сборка Docker-образа
```

Для `make test-integration` сначала запустите окружение через
`tripgoctl environment start`. Тесты используют `DATABASE_URL` из `.env`.
Каждый тест применяет миграции в своей временной схеме и удаляет ее при
завершении. Для этой команды не нужно заранее выполнять `make migrate`.

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
| `OTEL_SERVICE_NAME` | имя сервиса в телеметрии |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | адрес коллектора из `.env` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | протокол `http/protobuf` или `grpc` |
| `OTEL_RESOURCE_ATTRIBUTES` | атрибуты ресурса OpenTelemetry |
| `OTEL_TRACES_SAMPLER` | режим сэмплирования трейсов |
| `OTEL_METRIC_EXPORT_INTERVAL` | интервал экспорта метрик в миллисекундах |

`Makefile` загружает безопасные значения из `.env.example`, затем применяет
значения локального окружения из созданного `tripgoctl` файла `.env`.

## HTTP API

| Метод | Путь | Назначение |
|---|---|---|
| `POST` | `/api/v1/trips` | создать поездку |
| `GET` | `/api/v1/trips/{tripId}` | получить поездку |
| `POST` | `/api/v1/trips/{tripId}/finish` | завершить поездку |
| `GET` | `/health` | проверить процесс |
| `GET` | `/ready` | проверить PostgreSQL |

Источник контракта: `contracts/openapi/trip-service.openapi.yaml`.
Сгенерированный файл: `internal/generated/api/api.gen.go`.

## Архитектура

- `cmd/trip-service` читает конфигурацию и управляет запуском и остановкой.
  Конкретные реализации связываются в `internal/container`.
- `internal/http` содержит отдельные обработчики создания, чтения и
  завершения поездки. Перевод ошибок в HTTP-ответы собран в `domainerror`.
- `internal/usecase/trip` содержит сценарии работы с поездкой.
  Хендлеры и сценарии объявляют нужные им интерфейсы в своих `contract.go`.
- `internal/domain/trip` содержит модель поездки, ее состояния, правила
  переходов, валидацию и доменные ошибки.
- `internal/repository/trip` выполняет SQL через менеджер транзакций из
  `internal/infra/postgres/transaction`. Репозиторий получает исполнитель
  запросов через свой интерфейс.
- `internal/infra/telemetry` настраивает OpenTelemetry и JSON-логи. Обертка
  над сценариями создания и завершения записывает бизнес-события после коммита.

## Наблюдаемость

Откройте Jaeger и Grafana по ссылкам из `tripgoctl connect`. Отправьте запрос
на создание поездки и найдите в JSON-логе `trace_id`. По нему в Jaeger можно
открыть HTTP-спан и вложенные спаны PostgreSQL. В Grafana Explore выберите
Prometheus и запросите `http_server_request_duration_seconds_count` и
`tripgo_trips_created_total`.

В логах, метриках и именах HTTP-спанов используется шаблон маршрута.
Повтор создания с тем же ключом идемпотентности не увеличивает счетчик поездок.

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

Доменный метод `Trip.Finish` меняет состояние поездки. Репозиторий сохраняет
изменение через `UPDATE` с условием `status = 'active'`. Поэтому только один
конкурентный запрос изменяет поездку. Остальные получают `409 trip_completed`.
Запись истории входит в ту же транзакцию.

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
  --env-file .env.example \
  --env-file .env \
  -e DATABASE_URL="${DATABASE_URL/localhost/host.docker.internal}" \
  -e OTEL_EXPORTER_OTLP_ENDPOINT="${OTEL_EXPORTER_OTLP_ENDPOINT/localhost/host.docker.internal}" \
  -p 8080:8080 \
  trip-service:local
```
