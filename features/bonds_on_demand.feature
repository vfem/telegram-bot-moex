Feature: On-Demand Bond Information and Payment Queries
  As an investor using Telegram
  I want to query Russian bond details, payment schedules, and grouped portfolios
  So that I can track my bond investments across Moscow and SPB exchanges

  Background:
    Given the bot is running with market data for MOEX and SPB Exchange

  Scenario: Request bond passport and upcoming payments by ISIN
    When the user requests bond details for "RU000A107456"
    Then the bot should return a bond passport with:
      | Field          | Value                   |
      | Name           | ЕвроТранс БО-001Р-03    |
      | Exchange       | MOEX                    |
      | Nominal        | 1000                    |
      | CouponRate     | 13.5                    |
    And the response should list upcoming coupon payments

  Scenario: Request bond info by issuer name search
    When the user searches for bond "ОФЗ 26238"
    Then the bot resolves the search to ISIN "SU26238RMFS4"
    And the bond exchange is identified as "MOEX"

  Scenario: Request all payments scheduled for a specific date
    Given the following payments are scheduled:
      | ISIN         | BondName            | Date       | Amount | Type   |
      | SU26238RMFS4 | ОФЗ 26238           | 2026-10-15 | 35.40  | COUPON |
      | RU000A107456 | ЕвроТранс БО-001Р-03| 2026-10-15 | 33.29  | COUPON |
      | RU000A105X64 | Делимобиль 1Р-02    | 2026-10-16 | 31.66  | COUPON |
    When the user queries payments for date "2026-10-15"
    Then the bot response should contain 2 payments:
      | BondName            | Amount |
      | ОФЗ 26238           | 35.40  |
      | ЕвроТранс БО-001Р-03| 33.29  |
    And the response should not contain "Делимобиль 1Р-02"

  Scenario: Query payments for a custom user bond group
    Given the user has created a group named "Высокодоходные" with bonds:
      | ISIN         |
      | RU000A107456 |
      | RU000A105X64 |
    When the user requests upcoming payments for group "Высокодоходные"
    Then the bot returns payments scheduled only for the bonds in "Высокодоходные"
