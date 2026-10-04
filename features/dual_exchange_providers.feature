Feature: Dual Exchange Market Data Providers (MOEX and SPBE)
  As the market data engine
  I need to fetch and normalize bond data from both Moscow and SPB exchanges without authentication
  So that users receive unified, accurate bond details regardless of trading venue

  Scenario: Fetch and parse MOEX ISS bond data with coupons
    Given a mock MOEX ISS response for security "SU26238RMFS4" with coupon schedule
    When the MOEX provider fetches bond "SU26238RMFS4"
    Then the returned bond exchange should be "MOEX"
    And the bond name should be "ОФЗ 26238"
    And the bond should have 2 scheduled payments

  Scenario: Ingest SPB Exchange unauthenticated public listing
    Given an SPB Exchange securities CSV containing:
      | ISIN         | Ticker   | Name                    | Currency |
      | RU000A105X64 | CARM-01  | КарМани выпуск 1        | RUB      |
    When the SPB Exchange provider loads the securities registry
    Then searching for "RU000A105X64" yields exchange "SPBE"

  Scenario: Composite provider resolves bond seamlessly across exchanges
    Given the MOEX provider has bond "SU26238RMFS4"
    And the SPB Exchange provider has bond "RU000A105X64"
    When the composite provider searches for "SU26238RMFS4"
    Then the resolved bond exchange is "MOEX"
    When the composite provider searches for "RU000A105X64"
    Then the resolved bond exchange is "SPBE"
