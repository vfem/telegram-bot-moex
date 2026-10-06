# FEAT-01: Real-Time Market Data & Trading Liquidity (Котировки и ликвидность)

> **Feature ID:** `FEAT-01`  
> **Component:** `internal/provider/moex`, `internal/domain`, `internal/telegram`  
> **Priority:** P0 (Must Have)  
> **Target Cost:** $0.00 / month (MOEX ISS Public API)  

---

## 1. Описание и User Story

**Как** инвестор, рассматривающий покупку или продажу облигации,  
**Я хочу** видеть в карточке инструмента актуальную рыночную цену (% и ₽), доходность к погашению (YTM), дневной оборот торгов и оценку ликвидности (спред),  
**Чтобы** понимать, по какой реальной цене я могу совершить сделку в приложении брокера и не застряну ли я в неликвидной бумаге (особенно актуально для ВДО).

---

## 2. Архитектура и источники данных

- **Источник:** MOEX ISS API (Московская биржа).
- **Эндпоинт котировок:**
  ```http
  GET https://iss.moex.com/iss/engines/stock/markets/bonds/securities/{SECID}.json?iss.meta=off
  ```
- **Эндпоинт режима торгов (Board Marketdata):**
  ```http
  GET https://iss.moex.com/iss/engines/stock/markets/bonds/boards/{BOARDID}/securities/{SECID}.json?iss.meta=off
  ```
  Основной режим торгов корпоративными и ОФЗ облигациями:
  - `TQCB` — корпоративные и региональные облигации (Т+1 / Т+2).
  - `TQOB` — государственные облигации (ОФЗ).
  - `TQOD` — облигации в иностранной валюте / замещающие.

### Таблица полей MOEX ISS `marketdata`:
| Поле MOEX ISS | Тип | Описание |
|---|---|---|
| `LAST` / `PREVLEGALCLOSEPRICE` | `float64` | Текущая цена (% от номинала) |
| `YIELD` / `YIELDTOOFFER` | `float64` | Эффективная доходность к погашению / оферте (% годовых) |
| `DURATION` | `int` | Дюрация выпуска (в днях) |
| `VOLTODAY` | `int64` | Объем торгов за торговую сессию (в штуках ценных бумаг) |
| `VALTODAY` | `float64` | Оборот за день в рублях |
| `NUMTRADES` | `int` | Количество сделок за текущую сессию |
| `BID` / `OFFER` | `float64` | Лучшая цена покупки / продажи в стакане |
| `SPREAD` | `float64` | Спред между лучшей покупкой и продажей |

---

## 3. Декомпозиция подзадач (Sub-tasks)

### `TASK-01-01`: Модель данных `MarketData` и интерфейс провайдера
- **Цель:** Определить типизированную структуру рыночных данных в доменном слое.
- **Файл:** `internal/domain/market_data.go`
- **Структура:**
  ```go
  package domain

  import "time"

  type LiquidityLevel string

  const (
      LiquidityHigh   LiquidityLevel = "Высокая"
      LiquidityMedium LiquidityLevel = "Средняя"
      LiquidityLow    LiquidityLevel = "Низкая"
      LiquidityIlliquid LiquidityLevel = "Неликвид"
  )

  type MarketData struct {
      ISIN            string         `json:"isin"`
      BoardID         string         `json:"board_id"`
      LastPricePct    float64        `json:"last_price_pct"`   // % от номинала
      LastPriceRub    float64        `json:"last_price_rub"`   // Чистая цена в рублях
      AccruedCoupon   float64        `json:"accrued_coupon"`   // НКД (накопленный купонный доход)
      FullPriceRub    float64        `json:"full_price_rub"`   // Грязная цена (цена + НКД)
      YTM             float64        `json:"ytm"`              // Доходность к погашению, %
      DurationDays    int            `json:"duration_days"`    // Дюрация, дни
      VolumeTodayRub  float64        `json:"volume_today_rub"` // Оборот в рублях
      TradesCount     int            `json:"trades_count"`     // Число сделок
      BidPricePct     float64        `json:"bid_price_pct"`
      OfferPricePct   float64        `json:"offer_price_pct"`
      SpreadPct       float64        `json:"spread_pct"`
      Liquidity       LiquidityLevel `json:"liquidity"`
      TradingStatus   string         `json:"trading_status"`   // "Торгуется", "Торги закрыты"
      UpdatedAt       time.Time      `json:"updated_at"`
  }
  ```
- **Критерий приемки (DoD):** Структура покрыта юнит-тестами сериализации в JSON.

---

### `TASK-01-02`: Клиент MOEX ISS для извлечения `marketdata`
- **Цель:** Расширить `internal/provider/moex/client.go` методом получения котировок.
- **Логика работы:**
  1. Определение активного режима торгов (`primary_boardid`, например `TQCB` или `TQOB`).
  2. Запрос к `iss/engines/stock/markets/bonds/boards/{board}/securities/{secid}.json`.
  3. Парсинг блока `marketdata` (с fallback на `securities.PREVLEGALCLOSEPRICE`, если рынок закрыт или сделок сегодня еще не было).
- **Критерий приемки (DoD):** Метод `GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error)` корректно возвращает цену и объем для ОФЗ и корпоративных бумаг в мок-тестах.

---

### `TASK-01-03`: Алгоритм классификации ликвидности и расчета спреда
- **Цель:** Вычислить спред и присвоить бумаге индикатор ликвидности для предотвращения рисков инвестора.
- **Алгоритм:**
  - `SpreadPct = OfferPricePct - BidPricePct`
  - Правила присвоения `LiquidityLevel`:
    - **Высокая (🔥):** Оборот > 10 млн ₽ в день И сделок > 100 И спред < 0.5%.
    - **Средняя (⚡️):** Оборот от 1 до 10 млн ₽ в день И сделок > 20 И спред < 1.5%.
    - **Низкая (⚠️):** Оборот от 100 тыс. до 1 млн ₽ ИЛИ спред > 1.5%.
    - **Неликвид (🛑):** Оборот < 100 тыс. ₽ за день ИЛИ 0 сделок.
- **Критерий приемки (DoD):** Алгоритм покрыт тестами на граничные значения оборота и спреда.

---

### `TASK-01-04`: Кэширующий слой `MarketDataCache`
- **Цель:** Исключить избыточные запросы к MOEX ISS при частых запросах одной и той же бумаги.
- **Файл:** `internal/store/memory/market_cache.go`
- **Параметры:**
  - TTL кэша: **120 секунд** (в часы торгов 09:50 – 18:50 МСК).
  - TTL кэша: **60 минут** (вне торговых часов и в выходные дни).
- **Критерий приемки (DoD):** Повторный запрос в пределах 2 минут не инициирует HTTP-вызов к MOEX ISS.

---

### `TASK-01-05`: Telegram-форматтер и обновление `/bond`
- **Цель:** Интегрировать блок рыночных данных в выдачу карточки инструмента.
- **Файл:** `internal/telegram/formatters.go`
- **Вид карточки:**
  ```text
  📈 Рынок и торги:
  • Цена: 98.40% (984.00 ₽) • НКД: 14.20 ₽
  • YTM: 21.45% • Дюрация: 420 дн. (~1.2 г.)
  • Оборот сегодня: 14.8 млн ₽ (382 сделки)
  • Ликвидность: 🔥 Высокая (спред: 0.15%)
  ```
- **Критерий приемки (DoD):** При отсутствии торгов отображается цена закрытия предыдущего дня с пометкой `(цена закр.)`.

---

### `TASK-01-06`: BDD-сценарий и приемочные тесты
- **Файл спецификации:** `features/bond_market_data.feature`
- **Сценарий:**
  ```gherkin
  Scenario: Query bond market data returns clean price, YTM and volume
    Given the MOEX provider has market quote for "RU000A107456":
      | LastPricePct   | 98.40    |
      | YTM            | 21.45    |
      | VolumeTodayRub | 14800000 |
      | TradesCount    | 382      |
    When the user requests bond details for "RU000A107456"
    Then the response should contain market price "98.40%"
    And the response should contain YTM "21.45%"
    And the response should display liquidity "Высокая"
  ```
- **Критерий приемки (DoD):** Тест успешно проходит в общем наборе `go test ./tests/bdd`.
