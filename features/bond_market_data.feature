Feature: Real-Time Market Data and Liquidity Assessment
  As an investor considering buying or selling a bond
  I want to see live market prices, YTM, daily volume, and a liquidity assessment
  So that I know the execution price and can avoid illiquid bonds

  Background:
    Given the bot is running with market data for MOEX and SPB Exchange

  Scenario: Query bond market data returns clean price, YTM, volume, and high liquidity
    Given the MOEX provider has market quote for "RU000A107456":
      | LastPricePct    | 98.40    |
      | AccruedCoupon   | 14.20    |
      | YTM             | 21.45    |
      | DurationDays    | 420      |
      | VolumeTodayRub  | 14800000 |
      | TradesCount     | 382      |
      | BidPricePct     | 98.30    |
      | OfferPricePct   | 98.45    |
      | SpreadPct       | 0.15     |
    When the user requests bond details for "RU000A107456"
    Then the bot response should contain market price "98.40%"
    And the bot response should contain clean price in RUB "984.00 ₽"
    And the bot response should contain YTM "21.45%"
    And the bot response should display liquidity "Высокая"
    And the bot response should contain trades count 382

  Scenario: Fallback to previous close price when trading session has no deals
    Given the MOEX provider has market quote for "SU26238RMFS4":
      | LastPricePct    | 0.00     |
      | PrevClosePrice  | 54.20    |
      | AccruedCoupon   | 33.80    |
      | YTM             | 17.85    |
      | DurationDays    | 2450     |
      | VolumeTodayRub  | 0        |
      | TradesCount     | 0        |
      | SpreadPct       | 0        |
    When the user requests bond details for "SU26238RMFS4"
    Then the bot response should contain market price "54.20%"
    And the bot response should indicate "(цена закр.)"
    And the bot response should display liquidity "нет сделок сегодня"

  Scenario: In-memory cache returns market data on repeated queries within TTL
    Given the MOEX provider has market quote for "RU000A107456":
      | LastPricePct    | 98.40    |
      | AccruedCoupon   | 14.20    |
      | YTM             | 21.45    |
      | VolumeTodayRub  | 14800000 |
      | TradesCount     | 382      |
      | SpreadPct       | 0.15     |
    When the user requests bond details for "RU000A107456"
    Then the market data for "RU000A107456" should be present in the cache
    When the provider quote for "RU000A107456" changes to 99.00%
    And the user requests bond details for "RU000A107456" within cache TTL
    Then the bot response should still contain cached price "98.40%"
