# FEAT-01: Real-Time Market Data & Trading Liquidity (Котировки и ликвидность)

> **Feature ID:** `FEAT-01`  
> **Component:** `internal/provider/moex`, `internal/domain`, `internal/store`, `internal/service`, `internal/telegram`  
> **Priority:** P0 (Must Have)  
> **Target Cost:** $0.00 / month (MOEX ISS Public API)  
> **Status:** ✅ Завершено (All 20 review findings addressed and verified)

---

## 1. Описание и User Story

**Как** инвестор, рассматривающий покупку или продажу облигации,  
**Я хочу** видеть в карточке инструмента актуальную рыночную цену (% и валюта), доходность к погашению (YTM), дневной оборот торгов и оценку ликвидности (спред),  
**Чтобы** понимать, по какой реальной цене я могу совершить сделку в приложении брокера и не застряну ли я в неликвидной бумаге (особенно актуально для ВДО).

---

## 2. Архитектура и источники данных

- **Источник:** MOEX ISS API (Московская биржа).
- **Эндпоинт котировок и спецификаций:**
  ```http
  GET https://iss.moex.com/iss/engines/stock/markets/bonds/securities/{SECID}.json?iss.meta=off
  ```
- **Эндпоинт режима торгов (Board Marketdata fallback):**
  ```http
  GET https://iss.moex.com/iss/engines/stock/markets/bonds/boards/{BOARDID}/securities/{SECID}.json?iss.meta=off
  ```
  Поддерживаемые режимы торгов:
  - `TQCB` — корпоративные и региональные облигации.
  - `TQOB` — государственные облигации (ОФЗ).
  - `TQIR` — корпоративные облигации для квалифицированных инвесторов.
  - `TQOD`, `TQOE` — облигации в иностранной валюте / внесистемные режимы.

### Таблица ключевых полей MOEX ISS:
| Таблица | Поле MOEX ISS | Тип | Описание |
|---|---|---|---|
| `securities` | `FACEVALUE` | `float64` | Номинальная стоимость бумаги |
| `securities` | `FACEUNIT` / `CURRENCYID` | `string` | Валюта номинала (`RUB`, `USD`, `EUR`, `CNY`) |
| `securities` | `ACCRUEDINT` | `float64` | НКД (накопленный купонный доход в валюте) |
| `securities` | `PREVLEGALCLOSEPRICE` | `float64` | Официальная цена закрытия предыдущего дня (% от номинала) |
| `marketdata` | `LAST` | `float64` | Цена последней сделки (% от номинала) |
| `marketdata` | `YIELD` | `float64` | Эффективная доходность к погашению (YTM, % годовых) |
| `marketdata` | `DURATION` | `int` | Дюрация выпуска (в днях) |
| `marketdata` | `VALTODAY` | `float64` | Оборот за день в рублях |
| `marketdata` | `NUMTRADES` | `int` | Количество сделок за сессию |
| `marketdata` | `BID` / `OFFER` | `float64` | Лучшая цена покупки / продажи в стакане (% от номинала) |
| `marketdata` | `TRADINGSTATUS` | `string` | Статус торгов (например, `T`, `N`) |

---

## 3. Ключевые архитектурные решения и особенности реализации (Design Decisions)

1. **Разрешение идентификаторов (Ticker-First with ISIN Fallback):**
   - На MOEX ISS для ОФЗ эндпоинты котировок и графиков выплат требуют `SECID` (тикер, например `SU26238RMFS4`), тогда как `ISIN` — `RU000A1038V6`. Для большинства корпоративных облигаций тикер и ISIN совпадают.
   - Сервис `BondService.GetBondDetails` сначала запрашивает данные по `bond.Ticker`, и в случае отсутствия обращается по `bond.ISIN`.
2. **Селекция режима торгов и сопоставление таблиц (Multi-Board Selection):**
   - При листинге на нескольких режимах клиент выбирает строку из `marketdata` с приоритетом активных торгов (`NUMTRADES > 0 || LAST > 0`) по режимам `TQCB`, `TQOB`, `TQIR`, `TQOD`, `TQOE`.
   - Затем строка спецификации из `securities` строго сопоставляется по `BOARDID == md.BoardID`, что исключает рассинхронизацию номинала или НКД между режимами.
3. **Мультивалютность (Multi-Currency):**
   - Распознаются валюты `RUB`, `USD`, `EUR`, `CNY` из полей `FACEUNIT`/`CURRENCYID`. Чистая и полная цена форматируются с соответствующим символом (`₽`, `$`, `€`, `¥`).
4. **Адаптивный TTL кэша с учетом вечерней сессии (Adaptive Cache TTL):**
   - Будни **09:50 – 23:50 МСК** (основная + вечерняя торговая сессия): **120 секунд**.
   - Ночные часы (после 23:50 до 09:50) и выходные: **60 минут**.
   - Реализована ленивая очистка (lazy eviction) и возврат копии структуры (`copy-on-read`), исключающий гонки мутации кэша.
   - Интерфейс `store.MarketDataCache` внедрен в `BondService`, отделяя логику от конкретной in-memory реализации.
5. **Оценка ликвидности вне торговой сессии (Off-Hours Liquidity):**
   - Если рынок закрыт или сегодня еще не было сделок (`TradesCount == 0 && IsPreviousClose`), ликвидность помечается как `⏳ нет сделок сегодня`, исключая ложный сигнал `🛑 Неликвид` для ликвидных бумаг (например, ОФЗ 26238 на выходных).
6. **Относительный спред и учет стакана котировок (Relative Spread & Quote Check):**
   - Относительный спред рассчитывается по формуле:
     $$\text{SpreadPct} = \frac{\text{Offer} - \text{Bid}}{\frac{\text{Offer} + \text{Bid}}{2}} \times 100\%$$
   - Наличие двухсторонней котировки (`HasQuote`): `BidPricePct > 0 && OfferPricePct > 0`.
   - Если двухсторонняя котировка отсутствует, оценка ликвидности ограничивается категорией `Низкая`, а в карточке выводится `(спред: н/д)`.
7. **Точное сопоставление и кэширование ИНН эмитента (INN Exact Match & Cache):**
   - При дообогащении метаданных эмитента через `/securities.json?q=` выбирается строка, у которой `secid` или `isin` совпадает с запрошенным инструментом.
   - Результат кэшируется в памяти `moex.Client`, исключая повторные HTTP-запросы при частых вызовах.
8. **Обработка пустых ответов ISS (Empty ISS Handling):**
   - При отсутствии инструмента MOEX ISS возвращает HTTP 200 с пустыми массивами `data: []`. Парсер возвращает `nil, nil`, что позволяет композитному провайдеру штатно перейти к проверке СПб Биржи (SPBE).
9. **Строгое экранирование Telegram MarkdownV2:**
   - Все 18 зарезервированных символов (`_ * [ ] ( ) ~ \ ` > # + - = | { } . !`) экранируются за пределами тегов разметки.
   - В `bot.go` при ошибке Telegram API (не 200) логируется тело ответа перед переходом на plain text fallback.
10. **Русская типографика и плавающие купоны:**
    - Дюрация выводится с десятичной запятой и родительным падежом: `~6,7 года`.
    - Неопределенные купонные выплаты (`Amount == 0`) отображаются как `_размер уточняется_`.

---

## 4. Публичный API и изменения контрактов (Public API Changes)

- **Интерфейс `provider.BondProvider`:**
  - Добавлен метод:
    ```go
    GetMarketData(ctx context.Context, identifier string) (*domain.MarketData, error)
    ```
- **Интерфейс `store.MarketDataCache`:**
  - Добавлен интерфейс в пакет `internal/store`:
    ```go
    type MarketDataCache interface {
        Get(isin string) (*domain.MarketData, bool)
        Set(isin string, data *domain.MarketData)
    }
    ```
- **Сервис `service.BondService`:**
  - Добавлен метод `GetBondDetails(ctx context.Context, identifier string) (*domain.Bond, *domain.MarketData, []domain.Payment, error)`.
  - Метод `GetBondWithPayments` сохранен как фасад для обратной совместимости.
