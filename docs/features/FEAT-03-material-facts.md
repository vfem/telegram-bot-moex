# FEAT-03: Material Facts & Corporate Events Monitoring (Существенные факты и события)

> **Feature ID:** `FEAT-03`  
> **Component:** `internal/provider/edisclosure`, `internal/domain`, `internal/service`, `internal/telegram`  
> **Priority:** P1 (Should Have)  
> **Target Cost:** $0.00 / month (Публичные ленты e-disclosure.ru и MOEX News)  

---

## 1. Описание и User Story

**Как** держатель или покупатель облигаций,  
**Я хочу** видеть последние существенные факты по эмитенту (оферты, выплаты купонов, решения СД/ВОСА, техдефолты, судебные споры) и получать срочные оповещения,  
**Чтобы** вовремя среагировать на приближение дефолта, не пропустить предъявление бумаг к оферте или узнать о важной корпоративной реорганизации.

---

## 2. Архитектура и источники данных

- **Источники:**
  1. **Интерфакс-ЦРКИ (e-disclosure.ru):** Официально аккредитованный Банком России сервер раскрытия информации. Содержит ленту сообщений по каждому ИНН эмитента.
  2. **MOEX News & Disclosures API:** Лента сообщений о ценных бумагах:
     ```http
     GET https://iss.moex.com/iss/sbc/news/securities/{SECID}.json
     ```
- **Типы событий (категоризация):**
  - 🚨 **Критические (Critical):** Неисполнение обязательств (дефолт / техдефолт), банкротство, отзыв лицензии, предъявление к досрочному погашению.
  - ⚠️ **Предупреждающие (Warning):** Оферта (Put/Call), изменение ставки купона, крупные судебные иски (> 10% чистых активов), смена руководства/контролирующего лица.
  - ℹ️ **Информационные (Info):** Решения совета директоров, созыв ГОСА/ВОСА, выплата купонного дохода в полном объеме, плановое раскрытие отчетности.

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
      FactDefault       FactCategory = "Дефолт / Обязательства"
      FactOffer         FactCategory = "Оферта"
      FactCoupon        FactCategory = "Купонные выплаты"
      FactMeeting       FactCategory = "СД и Собрания акционеров"
      FactLitigation    FactCategory = "Судебные споры"
      FactRatingChange  FactCategory = "Рейтинговые действия"
      FactOther         FactCategory = "Прочее"
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
      Title       string       `json:"title"`
      Category    FactCategory `json:"category"`
      Severity    FactSeverity `json:"severity"`
      PublishedAt time.Time    `json:"published_at"`
      URL         string       `json:"url"`
  }
  ```

---

### `TASK-03-02`: Провайдер ленты раскрытия информации
- **Цель:** Получение списка последних 5–10 существенных фактов по ИНН эмитента или коду инструмента.
- **Файл:** `internal/provider/edisclosure/client.go`
- **Метод:**
  ```go
  type FactsProvider interface {
      GetRecentFacts(ctx context.Context, inn string, limit int) ([]domain.MaterialFact, error)
  }
  ```
- **Логика:** Парсинг RSS/JSON-ленты раскрытия информации по компании.
- **Кэширование:** TTL **30–60 минут** (события появляются в течение дня).

---

### `TASK-03-03`: Движок классификации критичности и категорий
- **Цель:** Автоматический анализ заголовка и текста факта для присвоения `Severity` и `Category`.
- **Правила классификации:**
  - Ключевые слова `SeverityCritical`: "неисполнение обязательств", "технический дефолт", "банкротств", "приостановлени".
  - Ключевые слова `SeverityWarning`: "приобретении ценных бумаг" (оферта), "требован", "иск", "арест", "оферт".
  - Ключевые слова `SeverityInfo`: "выплата дохода", "решения совета директоров", "созыв".
- **Критерий приемки (DoD):** Модульные тесты подтверждают 100% точность классификации критических дефолтных событий.

---

### `TASK-03-04`: Сервис отображения фактов и команда `/facts <ISIN>`
- **Цель:** Предоставить пользователю возможность запросить историю событий по выпуску.
- **Файлы:** `internal/service/facts_service.go`, `internal/telegram/bot.go`
- **Вид вывода:**
  ```text
  ⚠️ Существенные факты: ПАО «ЕвроТранс» (ИНН 7708304940)

  • 02.10.2026 14:15 [ℹ️ Купон]:
    Выплата купонного дохода по облигациям серии БО-001Р-03 в полном объеме
    🔗 [Подробнее на e-disclosure](...)

  • 25.09.2026 18:30 [⚠️ Оферта]:
    Определение ставки купона и даты начала периода предъявления бумаг к выкупу
    🔗 [Подробнее](...)

  • 12.08.2026 10:00 [ℹ️ СД]:
    Решения заседания совета директоров эмитента
  ```

---

### `TASK-03-05`: Интеграция проактивных алертов в `/sub add`
- **Цель:** Если пользователь добавил бумагу в отслеживаемые (`/sub add <ISIN>`), бот при обнаружении событий со статусом `CRITICAL` или `WARNING` немедленно присылает push-уведомление.
- **Файл:** `internal/service/notifier_service.go`
- **Формат уведомления:**
  ```text
  🚨 ВНИМАНИЕ: Срочное событие по вашей облигации!
  Облигация: Озон 1Р-01 (RU000A105...)

  ⚠️ Тип: Оферта (Выкуп бумаг)
  Дата события: 15.10.2026
  Суть: Начался период подачи заявок на выкуп по номиналу.
  ```

---

### `TASK-03-06`: BDD-сценарии и приемочные тесты
- **Файл спецификации:** `features/material_facts.feature`
- **Сценарий:**
  ```gherkin
  Scenario: Critical default fact triggers warning severity
    Given a new disclosure fact for issuer INN "7708304940":
      | Title       | Неисполнение обязательств эмитента по выплате купона |
      | PublishedAt | 2026-10-06                                           |
    When the facts classifier processes the announcement
    Then the severity should be "CRITICAL"
    And the category should be "Дефолт / Обязательства"
  ```
