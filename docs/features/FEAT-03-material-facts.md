# FEAT-03: Material Facts & Corporate Events Monitoring (Существенные факты и события)

> **Feature ID:** `FEAT-03`  
> **Component:** `internal/provider/edisclosure`, `internal/domain`, `internal/service`, `internal/telegram`  
> **Priority:** P1 (Should Have)  
> **Target Cost:** $0.00 / month (Публичные API MOEX ISS Bondization & SiteNews / ленты e-disclosure.ru)  

---

## 1. Описание и User Story

**Как** держатель или покупатель облигаций,  
**Я хочу** видеть последние существенные факты по эмитенту (оферты, выплаты купонов, решения СД/ВОСА, техдефолты, судебные споры) и получать срочные оповещения,  
**Чтобы** вовремя среагировать на приближение дефолта, не пропустить предъявление бумаг к оферте или узнать о важной корпоративной реорганизации.

---

## 2. Архитектура и источники данных

- **Источники:**
  1. **MOEX ISS Bondization & Securities API (Основной публичный источник без WAF-блокировок):**
     ```http
     GET https://iss.moex.com/iss/securities.json?q={INN}&iss.meta=off&group_by=group&group_by_filter=stock_bonds&limit=10
     GET https://iss.moex.com/iss/securities/{SECID}.json?iss.meta=off
     GET https://iss.moex.com/iss/securities/{SECID}/bondization.json?iss.meta=off&limit=100
     GET https://iss.moex.com/iss/sitenews.json?iss.meta=off
     ```
     Извлекает статус дефолта/техдефолта (`HASDEFAULT`, `HASTECHNICALDEFAULT`), даты оферт (`offers`), выплаченные купоны и предстоящие определения ставок (`coupons`), амортизации (`amortizations`), а также новости биржи (`sitenews`), отфильтрованные строго по эмитенту/тикеру/ISIN.
  2. **Интерфакс-ЦРКИ (e-disclosure.ru) / RSS 2.0 & JSON:** Поддержка разбора XML RSS 2.0 и JSON-массивов корпоративных сообщений.
- **Типы событий (категоризация):**
  - 🚨 **Критические (Critical):** Неисполнение обязательств (дефолт / техдефолт), банкротство, отзыв лицензии, приостановление торгов/эмиссии.
  - ⚠️ **Предупреждающие (Warning):** Оферта (Put/Call), определение ставки купона перед офертой, судебные иски и арбитражные споры, понижение/отзыв рейтинга.
  - ℹ️ **Информационные (Info):** Решения совета директоров, созыв ГОСА/ВОСА, выплата купонного дохода в полном объеме, амортизация, плановое подтверждение рейтинга.

---

## 3. Декомпозиция подзадач (Sub-tasks)

### `TASK-03-01`: Доменная модель `MaterialFact`
- **Файл:** `internal/domain/fact.go`
- **Структура:**
  ```go
  package domain

  import "time"

  type FactCategory string

  const (
      FactDefault      FactCategory = "Дефолт / Обязательства"
      FactOffer        FactCategory = "Оферта"
      FactCoupon       FactCategory = "Купонные выплаты"
      FactMeeting      FactCategory = "СД и Собрания акционеров"
      FactLitigation   FactCategory = "Судебные споры"
      FactRatingChange FactCategory = "Рейтинговые действия"
      FactOther        FactCategory = "Прочее"
  )

  type FactSeverity string

  const (
      SeverityCritical FactSeverity = "CRITICAL" // Красный флаг
      SeverityWarning  FactSeverity = "WARNING"  // Внимание
      SeverityInfo     FactSeverity = "INFO"     // Плановое событие
  )

  type MaterialFact struct {
      ID          string       `json:"id"`
      INN         string       `json:"inn"`
      ISIN        string       `json:"isin,omitempty"`
      IssuerName  string       `json:"issuer_name,omitempty"`
      Title       string       `json:"title"`
      Summary     string       `json:"summary,omitempty"`
      Category    FactCategory `json:"category"`
      Severity    FactSeverity `json:"severity"`
      PublishedAt time.Time    `json:"published_at"`
      URL         string       `json:"url"`
      Source      string       `json:"source,omitempty"`
  }
  ```

---

### `TASK-03-02`: Провайдер ленты раскрытия информации
- **Цель:** Получение списка последних 5–10 существенных фактов по ИНН эмитента.
- **Файл:** `internal/provider/edisclosure/client.go`
- **Метод:**
  ```go
  type FactsProvider interface {
      GetRecentFacts(ctx context.Context, inn string, limit int) ([]domain.MaterialFact, error)
  }
  ```
- **Логика:** Поиск выпусков эмитента по ИНН в MOEX ISS, сбор событий `bondization` (оферты, купоны, амортизации), проверка флагов дефолта и фильтрация `sitenews`, а также поддержка RSS 2.0 XML и JSON-лент с очисткой HTML-тегов и парсингом дат в часовом поясе `MSK (UTC+3)`.
- **Кэширование:** TTL **45 минут** для положительных ответов и **15 минут** для негативного кэша (`SetNotFound` / `IsNotFound`).

---

### `TASK-03-03`: Движок классификации критичности и категорий
- **Цель:** Автоматический анализ заголовка и текста факта для присвоения `Severity` и `Category` с учетом границ слов (`containsWord` и `containsStem`) для защиты от ложных срабатываний на подстроках (например, «тран**спор**т», «**риск**а», «ко**нкр**етных», «приостановлении полномочий»).
- **Критерий приемки (DoD):** Модульные тесты подтверждают 100% точность классификации критических дефолтных событий и отсутствие ложноположительных срабатываний.

---

### `TASK-03-04`: Сервис отображения фактов и команда `/facts <ISIN>` (алиас `/f`)
- **Цель:** Предоставить пользователю возможность запросить историю событий по выпуску, тикеру или ИНН.
- **Файлы:** `internal/service/facts_service.go`, `internal/telegram/bot.go`, `internal/telegram/formatters.go`
- **Вид вывода:**
  ```text
  ⚠️ Существенные факты: ПАО «ЕвроТранс» (ИНН 7708304947)

  • 02.10.2026 14:15 [ℹ️ Купон]: Выплата купонного дохода по облигациям серии БО-001Р-03 в полном объеме
    🔗 Подробнее
  • 25.09.2026 18:30 [⚠️ Оферта]: Определение ставки купона и даты начала периода предъявления бумаг к выкупу
    🔗 Подробнее
  • 12.08.2026 [ℹ️ СД]: Решения заседания совета директоров эмитента
  ```

---

### `TASK-03-05`: Интеграция проактивных алертов в `/sub add`
- **Цель:** Если пользователь добавил бумагу в отслеживаемые (`/sub add <ISIN>`), бот при обнаружении событий со статусом `CRITICAL` или `WARNING` присылает push-уведомление (вызывается при проверке `CheckMonitoredFacts` / `RunDailyDigest` и `NotifyMaterialFact`).
- **Файл:** `internal/service/notifier_service.go`
- **Формат уведомления:**
  ```text
  🚨 ВНИМАНИЕ: Срочное событие по вашей облигации!
  Облигация: ЕвроТранс БО-001Р-03 (RU000A107456)
  Эмитент: ПАО «ЕвроТранс»

  ⚠️ Тип: Оферта [WARNING]
  Дата события: 15.10.2026
  Суть: Начался период подачи заявок на выкуп по оферте
  ```

---

### `TASK-03-06`: BDD-сценарии и приемочные тесты
- **Файл спецификации:** `features/material_facts.feature` (10 сценариев)
- **Сценарий:**
  ```gherkin
  Scenario: Critical default fact triggers critical severity
    Given a new disclosure fact for issuer INN "7708304947":
      | Title       | Неисполнение обязательств эмитента по выплате купона |
      | PublishedAt | 2026-10-06                                           |
    When the facts classifier processes the announcement
    Then the severity should be "CRITICAL"
    And the category should be "Дефолт / Обязательства"
  ```

---

## 4. Заметки о реализации (Implementation Notes — ✅ Завершено)

- **Статус:** Все задачи `TASK-03-01` .. `TASK-03-06` реализованы и покрыты модульными, интеграционными (`TestLiveMaterialFacts`) и BDD-тестами (`features/material_facts.feature`, 10 сценариев).
- **Безопасность, надежность и защита ресурсов:**
  - В `edisclosure.Client` (`internal/provider/edisclosure/client.go`) встроена защита от SSRF при редиректах, ограничение ответа `io.LimitReader` до 4 МБ, санитизация URL (`SanitizeURL`), очистка HTML-тегов и парсинг дат в `Europe/Moscow (MSK)`.
  - В `FactsService` (`internal/service/facts_service.go`) используется обобщенная группа `singleflightGroup` с защитой от отмены контекста первого вызывающего (`context.WithoutCancel`), негативное кэширование (`SetNotFound` на 15 минут) и специальная обработка суверенных выпусков ОФЗ (`7710168360`).
  - В `NotifierService` (`internal/service/notifier_service.go`) добавлена потокобезопасная дедупликация алертов (`sync.Mutex`, лимит 10 000 записей, фиксация ключа только после успешной отправки в Telegram) и экранирование символов `MarkdownV2`.
