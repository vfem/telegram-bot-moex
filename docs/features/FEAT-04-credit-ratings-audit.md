# FEAT-04: Credit Ratings & Audit Quality Analysis (Рейтинги и аудит)

> **Feature ID:** `FEAT-04`  
> **Component:** `internal/provider/ratings`, `internal/domain`, `internal/telegram`  
> **Priority:** P1 (Should Have)  
> **Target Cost:** $0.00 / month (Публичные реестры ЦБ РФ, АКРА, Эксперт РА, ГИР БО)  

---

## 1. Описание и User Story

**Как** консервативный или умеренный инвестор,  
**Я хочу** видеть официальный кредитный рейтинг эмитента от аккредитованных агентств (АКРА, Эксперт РА, НКР, НРА), прогноз по нему, а также статус аудиторского заключения (чистое или с оговорками),  
**Чтобы** отсекать ненадежных эмитентов с рисками банкротства или фальсификации отчетности еще до покупки бумаг.

---

## 2. Архитектура и источники данных

1. **Кредитные рейтинги:**
   - Четыре официально аккредитованных ЦБ РФ кредитных рейтинговых агентства (КРА):
     - **АКРА (ACRA)** — национальная шкала (`AAA(RU)`, `AA+(RU)`, ..., `BBB-(RU)`, ..., `C(RU)`).
     - **Эксперт РА (RAEX)** — шкала (`ruAAA`, `ruAA+`, ..., `ruBBB`, ..., `ruD`).
     - **НКР** — шкала (`AAA.ru`, `AA+.ru`).
     - **НРА** — шкала (`AAA|ru|`).
   - Метаданные MOEX ISS: MOEX транслирует присвоенный рейтинг в карточке листинга ценной бумаги и эмитента.
2. **Аудиторское заключение:**
   - Извлекается из карточки годовой отчетности ГИР БО (ФНС):
     - Наименование аудиторской организации (например, Б1, Kept, ТеДо, Мариллион, Юникон и др.).
     - Мнение аудитора:
       - ✅ *Безоговорочно положительное* (чистое) — высший стандарт доверия.
       - ⚠️ *С оговорками* (модифицированное) — повод для настороженности.
       - 🛑 *Отрицательное* или *Отказ от выражения мнения* — критический стоп-фактор.

---

## 3. Декомпозиция подзадач (Sub-tasks)

### `TASK-04-01`: Доменные структуры `CreditRating` и `AuditInfo`
- **Файл:** `internal/domain/rating.go`
- **Структура:**
  ```go
  package domain

  import "time"

  type RatingGrade string

  type CreditRating struct {
      Agency      string      `json:"agency"`      // "АКРА", "Эксперт РА", "НКР", "НРА"
      Rating      string      `json:"rating"`      // "ruA-", "A+(RU)"
      Outlook     string      `json:"outlook"`     // "Стабильный", "Позитивный", "Негативный"
      AssignedAt  time.Time   `json:"assigned_at"`
      IsInvestmentGrade bool  `json:"is_investment_grade"` // true для BBB- и выше
  }

  type AuditOpinionType string

  const (
      AuditUnmodified           AuditOpinionType = "Безоговорочно положительное"
      AuditModifiedWithReserv   AuditOpinionType = "С оговорками"
      AuditAdverse              AuditOpinionType = "Отрицательное"
      AuditDisclaimer           AuditOpinionType = "Отказ от выражения мнения"
  )

  type AuditInfo struct {
      AuditorName string           `json:"auditor_name"`
      AuditorINN  string           `json:"auditor_inn,omitempty"`
      Year        int              `json:"year"`
      Opinion     AuditOpinionType `json:"opinion"`
      HasWarning  bool             `json:"has_warning"`
  }
  ```

---

### `TASK-04-02`: Провайдер извлечения рейтинга из MOEX и реестров
- **Цель:** Получение текущего актуального кредитного рейтинга эмитента/выпуска.
- **Файл:** `internal/provider/ratings/client.go`
- **Логика:**
  - Извлечение поля `credit_rating` из расширенного справочника MOEX ISS.
  - Fallback на открытый реестр рейтингов по ИНН эмитента.
- **Кэширование:** TTL **7 дней** (рейтинги пересматриваются не чаще нескольких раз в год).

---

### `TASK-04-03`: Парсинг аудиторского заключения из ГИР БО (ФНС)
- **Цель:** Автоматическое извлечение блока аудитора из отчета ФНС.
- **Файл:** `internal/provider/bfo/audit_parser.go`
- **Логика:**
  - Чтение секции `auditOrganization` и `opinionType` из JSON-ответа ФНС ГИР БО.
  - Определение флага `HasWarning`: взводится в `true`, если мнение отличается от безоговорочно положительного.

---

### `TASK-04-04`: Нормализация шкалы инвестиционной надежности
- **Цель:** Сопоставление шкал разных агентств к единой шкале надежности инвестора.
- **Шкала надежности:**
  - ⭐️ **Высочайшая (AAA):** ОФЗ, квазигосударственные гиганты (Сбер, Газпром).
  - 🔹 **Высокая (AA - A):** Крупный надежный бизнес.
  - 🔸 **Умеренная (BBB):** Нижняя граница инвестиционного грейда.
  - ⚡️ **Высокодоходные (BB - B):** ВДО, повышенная доходность и риск.
  - 🛑 **Дефолтные / Преддефолтные (CCC - D):** Спекулятивный / мусорный уровень.

---

### `TASK-04-05`: BDD-сценарии и приемочные тесты
- **Файл спецификации:** `features/credit_ratings.feature`
- **Сценарий:**
  ```gherkin
  Scenario: Display credit rating and audit opinion
    Given bond "RU000A107456" has credit rating:
      | Agency   | Эксперт РА |
      | Rating   | ruA-       |
      | Outlook  | Стабильный |
    And the latest audit for INN "7708304940" was issued by "Мариллион":
      | Opinion  | Безоговорочно положительное |
    When the user requests rating details for "RU000A107456"
    Then the response should show rating "ruA- (Эксперт РА) • Стабильный"
    And the response should show audit "Мариллион (Чистое)"
  ```
