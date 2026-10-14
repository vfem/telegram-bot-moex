Feature: Issuer Financial Statements and Balance Sheet Assessment
  As a bond investor
  I want to inspect key balance sheet figures (assets, equity, net debt) and debt-to-equity ratio
  So that I can quickly assess the solvency and default risk of the issuer before trading

  Background:
    Given the bot is running with financial reporting support

  Scenario: Query issuer balance sheet by bond ISIN
    Given bond "RU000A107456" has issuer INN "7708304947" and name "ПАО ЕвроТранс"
    And the financial provider has reports for INN "7708304947":
      | ReportYear      | 2025         |
      | TotalAssets     | 114200000000 |
      | Equity          | 38500000000  |
      | LongTermDebt    | 40000000000  |
      | ShortTermDebt   | 18100000000  |
      | CashEquivalents | 8800000000   |
      | Revenue         | 126400000000 |
      | NetProfit       | 4800000000   |
      | AuditorName     | ООО Мариллион|
    When the user sends command "/balance RU000A107456"
    Then the bot response should contain "114.2 млрд ₽"
    And the bot response should contain "38.5 млрд ₽"
    And the bot response should contain "49.3 млрд ₽"
    And the bot response should contain ratio "1.28"
    And the bot response should contain risk evaluation "Умеренная нагрузка"
    And the bot response should contain auditor "ООО Мариллион"

  Scenario: Query issuer balance sheet directly by INN
    Given the financial provider has reports for INN "7740000076":
      | ReportYear      | 2025         |
      | TotalAssets     | 1023000000000|
      | Equity          | 135200000000 |
      | LongTermDebt    | 377600000000 |
      | ShortTermDebt   | 166900000000 |
      | CashEquivalents | 16100000000  |
      | Revenue         | 391600000000 |
      | NetProfit       | 31300000000  |
    When the user sends command "/balance 7740000076"
    Then the bot response should contain "1.0 трлн ₽"
    And the bot response should contain "528.4 млрд ₽"
    And the bot response should contain risk evaluation "Высокая нагрузка"

  Scenario: Query balance using short alias /bal
    Given the financial provider has reports for INN "7740000076":
      | ReportYear      | 2025         |
      | TotalAssets     | 1023000000000|
      | Equity          | 135200000000 |
      | LongTermDebt    | 377600000000 |
      | ShortTermDebt   | 166900000000 |
      | CashEquivalents | 16100000000  |
      | Revenue         | 391600000000 |
      | NetProfit       | 31300000000  |
    When the user sends command "/bal 7740000076"
    Then the bot response should contain "1.0 трлн ₽"
    And the bot response should contain risk evaluation "Высокая нагрузка"

  Scenario: Negative equity indicates high debt risk
    Given the financial provider has reports for INN "7701234560":
      | ReportYear      | 2024        |
      | TotalAssets     | 10000000000 |
      | Equity          | -2000000000 |
      | LongTermDebt    | 12000000000 |
      | ShortTermDebt   | 3000000000  |
      | CashEquivalents | 500000000   |
    When the user sends command "/balance 7701234560"
    Then the bot response should contain "отриц. капитал"
    And the bot response should contain risk evaluation "Высокая нагрузка"

  Scenario: Cash-rich issuer with negative net debt shows net cash position
    Given the financial provider has reports for INN "7708304947":
      | ReportYear      | 2025         |
      | TotalAssets     | 50000000000  |
      | Equity          | 30000000000  |
      | LongTermDebt    | 5000000000   |
      | ShortTermDebt   | 2000000000   |
      | CashEquivalents | 15000000000  |
    When the user sends command "/balance 7708304947"
    Then the bot response should contain "Чистая ден. позиция"
    And the bot response should contain risk evaluation "Низкая нагрузка"

  Scenario: Credit organization reports explain reporting via Bank of Russia
    Given issuer INN "7707083893" is registered as a credit organization
    When the user sends command "/balance 7707083893"
    Then the bot response should contain "Кредитная организация"
    And the bot response should contain "Банка России"

  Scenario: Query balance for sovereign bond explains sovereign budget backing
    Given bond "SU26238RMFS4" has issuer INN "7710168360" and name "Министерство финансов РФ"
    When the user sends command "/balance SU26238RMFS4"
    Then the bot response should contain "Суверенный эмитент"
    And the bot response should contain "ОФЗ"
    And the bot response should contain "обеспечены суверенным бюджетом"

  Scenario: Regional finance ministry is not treated as federal sovereign
    Given bond "RU000A101UZ5" has issuer INN "5260000570" and name "Министерство финансов Нижегородской области"
    And the financial provider has no reports for INN "5260000570"
    When the user sends command "/balance RU000A101UZ5"
    Then the bot response should not contain "Суверенный эмитент"
    And the bot response should contain "не найден в базе ГИР БО"

  Scenario: Financials cache returns report within 30 days
    Given the financial provider has reports for INN "7708304947":
      | ReportYear      | 2025         |
      | TotalAssets     | 114200000000 |
      | Equity          | 38500000000  |
      | LongTermDebt    | 40000000000  |
      | ShortTermDebt   | 18100000000  |
      | CashEquivalents | 8800000000   |
    When the user sends command "/balance 7708304947"
    Then the financial statement for "7708304947" should be stored in the cache
    When the provider report for "7708304947" is cleared
    And the user sends command "/balance 7708304947"
    Then the bot response should contain "114.2 млрд ₽"

  Scenario: Rejection of invalid INN checksum
    When the user sends command "/balance 1234567890"
    Then the bot response should contain "Неверная контрольная сумма ИНН"

  Scenario: Reporting not found for valid commercial issuer
    Given the financial provider has no reports for INN "7701234560"
    When the user sends command "/balance 7701234560"
    Then the bot response should contain "не найден в базе ГИР БО"

  Scenario: Security rejection of injection and invalid inputs in balance command
    When the user sends command "/balance ../../etc/passwd"
    Then the bot response should contain "Неверный формат запроса"
    When the user sends command "/balance <script>alert(1)</script>"
    Then the bot response should contain "Неверный формат запроса"
