# STAGE-00: Sanity Check & Baseline Infrastructure Verification

> **Stage ID:** `STAGE-00`  
> **Component:** `cmd/bot`, `Dockerfile`, GCP Cloud Run, Cloudflare WIF Edge Proxy, Telegram Webhook, Public API Probers  
> **Priority:** P0 (Blocking Prerequisite)  
> **Objective:** Подтвердить работоспособность всех существующих движущихся частей системы перед реализацией новых фич.  
> **Status:** ✅ Завершено

---

## 1. Контекст и цели этапа

Перед тем как вносить изменения и добавлять новые модули (рыночные данные, баланс, факты), критически важно убедиться, что:
1. Текущий код собирается локально и пакуется в минимальный рабочий Docker-образ.
2. Внешние публичные API (MOEX ISS, SPB Exchange, bo.nalog.gov.ru) доступны по сети без сетевых ограничений и блокировок.
3. Бот успешно развернут в облаке (**Google Cloud Run**) в рамках бесплатного тарифа (\$0/мес).
4. Вебхук Telegram защищен секретным токеном (`X-Telegram-Bot-Api-Secret-Token`) и успешно обрабатывает входящие сообщения от Telegram API.
5. Сервер отдает статус `200 OK` на `/health` и `/healthz`, а Cloud Scheduler может триггерить `/cron/daily-digest`.

---

## 2. Диаграмма контура проверки (Moving Parts)

```
                            ┌───────────────────────────────────────────────┐
                            │            STAGE-00 SANITY CHECK              │
                            └───────────────────────┬───────────────────────┘
                                                    │
         ┌──────────────────┬───────────────────────┼───────────────────────┬──────────────────┐
         ▼                  ▼                       ▼                       ▼                  ▼
   [ 1. Build ]      [ 2. Public APIs ]      [ 3. Cloud Run ]       [ 4. Webhook ]       [ 5. Cron ]
  Go compile &       MOEX ISS & SPBE         Deploy container       Telegram Secret     Cloud Scheduler
  Docker < 20MB      network reachability    & env secrets check    token handshake     HTTP POST trigger
```

---

## 3. Декомпозиция подзадач (Sub-tasks)

### `TASK-00-01`: Проверка локальной сборки и контейнеризации
- **Цель:** Гарантировать чистую компиляцию Go-бинарника и валидность Docker-образа.
- **Действия:**
  1. Выполнить компиляцию:
     ```bash
     go build -v ./cmd/bot
     ```
  2. Проверить сборку Docker-образа:
     ```bash
     docker build -t moex-bonds-bot:sanity .
     ```
  3. Проверить размер итогового образа (требование: < 25 МБ благодаря Alpine и `CGO_ENABLED=0`).
- **Критерий приемки (DoD):** Бинарник и Docker-образ собираются без ошибок и предупреждений.

---

### `TASK-00-02`: Зондирование сетевой доступности внешних API (API Prober)
- **Цель:** Убедиться, что хост/контейнер имеет прямой беспрепятственный доступ к ключевым источникам данных.
- **Проверяемые эндпоинты:**
  - **MOEX ISS:**  
    `GET https://iss.moex.com/iss/securities/SU26238RMFS4.json?iss.meta=off` (статус 200, валидный JSON).
  - **MOEX Bondization:**  
    `GET https://iss.moex.com/iss/statistics/engines/stock/markets/bonds/bondization/SU26238RMFS4.json?iss.meta=off` (статус 200, купоны присутствуют).
  - **SPB Exchange Listing:**  
    `GET https://spbexchange.ru/ru/stocks/inostrannye/spisok.aspx` (публичный листинг доступен).
  - **ГИР БО (ФНС bo.nalog.gov.ru) - подготовка к MVP-2:**  
    `GET https://bo.nalog.gov.ru/` (сервер доступен по HTTPS без блокировок).
- **Критерий приемки (DoD):** Все HTTP-запросы возвращают `200 OK` с задержкой < 1.5 сек.

---

### `TASK-00-03`: Развертывание в GCP Cloud Run
- **Цель:** Развернуть сервис в продакшн-окружении Google Cloud Platform с нулевой стоимостью.
- **Команда деплоя:**
  ```bash
  gcloud run deploy moex-bonds-bot \
    --source . \
    --region europe-west1 \
    --platform managed \
    --no-allow-unauthenticated \
    --memory 128Mi \
    --cpu 1 \
    --min-instances 0 \
    --max-instances 2 \
    --set-secrets "TELEGRAM_BOT_TOKEN=telegram-bot-token:latest,TELEGRAM_SECRET_TOKEN=telegram-secret-token:latest"
  ```
- **Проверка здоровья (Health check):**
  ```bash
  curl -i https://<SERVICE_URL>/healthz
  # Ожидается: HTTP/2 200 OK, {"status":"healthy","service":"moex-spbe-bonds-bot"}
  ```
- **Критерий приемки (DoD):** Cloud Run возвращает HTTP 200 на `/healthz`.

---

### `TASK-00-04`: Настройка и защита вебхука Telegram
- **Цель:** Связать бота с Telegram Bot API и защитить вебхук от неавторизованного доступа.
- **Регистрация вебхука:**
  ```bash
  curl -X POST "https://api.telegram.org/bot<BOT_TOKEN>/setWebhook" \
    -d "url=https://<SERVICE_URL>/webhook" \
    -d "secret_token=<SECRET_TOKEN>" \
    -d "allowed_updates=[\"message\"]"
  ```
- **Тест безопасности (Security check):**
  - Запрос к `/webhook` **без** заголовка `X-Telegram-Bot-Api-Secret-Token` должен вернуть `401 Unauthorized` (проверка в постоянном времени `crypto/subtle`).
  - Запрос к `/webhook` **с неверным** заголовком должен вернуть `401 Unauthorized`.
  - Запрос к `/webhook` при заданном `TELEGRAM_BOT_TOKEN`, но отсутствующем `TELEGRAM_SECRET_TOKEN` возвращает `503 Service Unavailable` (fail-closed guardrail).
  - Запрос к `/webhook` **с корректным** секретным токеном должен вернуть `200 OK`.
  - Запрос методом, отличным от `POST` (например, `GET`), возвращает `405 Method Not Allowed`.
- **Критерий приемки (DoD):** Telegram подтверждает статус вебхука (`getWebhookInfo`), неавторизованные запросы блокируются.

---

### `TASK-00-05`: Автоматизированный скрипт Sanity Check
- **Цель:** Создать единую утилиту для повторной проверки системы в 1 клик.
- **Файл:** `scripts/sanity_check.ps1` (для Windows/PowerShell) и `scripts/sanity_check.sh` (для Linux/macOS).
- **Скрипт проверяет:**
  0. `[0]` Сканирование репозитория и git-истории на утечки секретов (`gitleaks`).
  1. `[1]` Прохождение BDD-тестов (`go test -count=1 ./tests/bdd`).
  2. `[2]` Компиляция бинарника (`go build ./cmd/bot`).
  3. `[3]` Доступность MOEX ISS API и живой интеграционный тест (`go test -tags=integration ./tests/integration`).
  4. `[4]` Валидность токена бота через `https://api.telegram.org/bot<TOKEN>/getMe` (опционально, если задан `TELEGRAM_BOT_TOKEN`).
  5. `[5]` Доступность удаленного эндпоинта `/health` / `/healthz` с OIDC-токеном (опционально, если задан `CLOUD_RUN_URL`).
  6. `[6]` Проверка защиты `/webhook`: отклонение запроса без секрета (`401/403`) и успешный ответ `200 OK` с заголовком `X-Telegram-Bot-Api-Secret-Token`.
- **Критерий приемки (DoD):** Скрипт выдает `✅ ALL SANITY CHECKS PASSED`.

---

### `TASK-00-06`: Проверка Cloud Scheduler (Daily Digest Cron)
- **Цель:** Убедиться, что расписание утренней рассылки купонов (09:00 МСК) может вызывать бэкенд.
- **Команда настройки задания:**
  ```bash
  gcloud scheduler jobs create http moex-daily-digest \
    --schedule="0 6 * * *" \
    --uri="https://<SERVICE_URL>/cron/daily-digest" \
    --http-method=POST \
    --oidc-service-account-email="scheduler-invoker@<PROJECT_ID>.iam.gserviceaccount.com" \
    --oidc-token-audience="https://<SERVICE_URL>"
  ```
- **Тестовый триггер:**
  ```bash
  gcloud scheduler jobs run moex-daily-digest
  ```
- **Безопасность:** Эндпоинт `/cron/daily-digest` принимает строго метод `POST` (при `GET` возвращает `405 Method Not Allowed`). Ошибки логируются на стороне сервера, клиент получает санированный статус `500 Internal server error` без раскрытия стека или деталей БД.
- **Критерий приемки (DoD):** Cloud Scheduler успешно отрабатывает без ошибок (код завершения 200 в логах Cloud Run).

---

## 4. Чек-лист готовности к переходу на MVP-1 (✅ Выполнено)

- [x] Все BDD тесты проходят локально (`30 scenarios, 156 steps passed`).
- [x] Сервер развернут на Cloud Run и доступен по HTTPS (за защитой Cloudflare WIF Edge Proxy).
- [x] MOEX ISS и ГИР БО ФНС отвечают актуальными данными.
- [x] Вебхук настроен с проверкой секретного токена (`X-Telegram-Bot-Api-Secret-Token`).
- [x] Бот ответил в реальном Telegram-чате на тестовую команду `/bond SU26238RMFS4`.
