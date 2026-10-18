Feature: Material Facts and Corporate Events Monitoring
  As a bond investor
  I want to inspect recent corporate disclosures (defaults, offers, coupon payments, board meetings, litigation) and receive urgent alerts
  So that I can react in time to put offers, technical defaults, or major corporate actions

  Background:
    Given the bot is running with financial reporting support

  Scenario: Critical default fact triggers critical severity
    Given a new disclosure fact for issuer INN "7708304947":
      | Title       | Неисполнение обязательств эмитента по выплате купона |
      | PublishedAt | 2026-10-06                                           |
    When the facts classifier processes the announcement
    Then the severity should be "CRITICAL"
    And the category should be "Дефолт / Обязательства"

  Scenario: Offer announcement triggers warning severity
    Given a new disclosure fact for issuer INN "7708304947":
      | Title       | Определение ставки купона и даты начала периода предъявления бумаг к выкупу |
      | PublishedAt | 2026-09-25                                                                  |
    When the facts classifier processes the announcement
    Then the severity should be "WARNING"
    And the category should be "Оферта"

  Scenario: Query recent material facts by bond ISIN
    Given bond "RU000A107456" has issuer INN "7708304947" and name "ПАО ЕвроТранс"
    And the disclosure provider has facts for INN "7708304947":
      | PublishedAt | Title                                                           | URL                                                 |
      | 2026-10-02  | Выплата купонного дохода в полном объеме                        | https://www.e-disclosure.ru/portal/event.aspx?id=1  |
      | 2026-09-25  | Определение ставки купона и даты выкупа                         | https://www.e-disclosure.ru/portal/event.aspx?id=2  |
      | 2026-08-12  | Решения заседания совета директоров                             |                                                     |
    When the user sends command "/facts RU000A107456"
    Then the bot response should contain "Существенные факты: ПАО ЕвроТранс"
    And the bot response should contain "7708304947"
    And the bot response should contain "[ℹ️ Купон]"
    And the bot response should contain "Выплата купонного дохода в полном объеме"
    And the bot response should contain "[⚠️ Оферта]"
    And the bot response should contain "Определение ставки купона и даты выкупа"
    And the bot response should contain "[ℹ️ СД]"
    And the bot response should contain "Решения заседания совета директоров"

  Scenario: Query material facts directly by INN using short alias /f
    Given the disclosure provider has facts for INN "7708304947":
      | PublishedAt | Title                                                           | URL |
      | 2026-10-05  | Возбуждение арбитражного судебного спора                        |     |
    When the user sends command "/f 7708304947"
    Then the bot response should contain "[⚠️ Суд]"
    And the bot response should contain "Возбуждение арбитражного судебного спора"

  Scenario: Query material facts for sovereign bond explains sovereign status
    Given bond "SU26238RMFS4" has issuer INN "7710168360" and name "Министерство финансов РФ"
    When the user sends command "/facts SU26238RMFS4"
    Then the bot response should contain "Суверенный эмитент"
    And the bot response should contain "ОФЗ"
    And the bot response should contain "не подлежат корпоративному раскрытию"

  Scenario: Query material facts when no disclosures exist for issuer
    Given the disclosure provider has no facts for INN "7701234560"
    When the user sends command "/facts 7701234560"
    Then the bot response should contain "свежих существенных фактов не найдено"

  Scenario: Facts cache serves subsequent requests within TTL
    Given the disclosure provider has facts for INN "7708304947":
      | PublishedAt | Title                                    | URL |
      | 2026-10-02  | Выплата купонного дохода в полном объеме |     |
    When the user sends command "/facts 7708304947"
    Then the material facts for "7708304947" should be stored in the cache
    When the disclosure provider facts for "7708304947" are cleared
    And the user sends command "/facts 7708304947"
    Then the bot response should contain "Выплата купонного дохода в полном объеме"

  Scenario: Proactive push alert is sent to user monitoring bond on critical or warning fact
    Given a clean subscription storage
    And a user with Telegram ID 55501
    And the user adds bond "RU000A107456" to their monitored bonds
    And bond "RU000A107456" has issuer INN "7708304947" and name "ЕвроТранс БО-001Р-03"
    When a material fact is detected for bond "RU000A107456":
      | Title       | Начался период подачи заявок на выкуп по оферте |
      | PublishedAt | 2026-10-15                                      |
    Then a notification message is queued for user 55501
    And the notification message contains "Срочное событие по вашей облигации"
    And the notification message contains "ЕвроТранс БО-001Р-03 (RU000A107456)"
    And the notification message contains "Оферта [WARNING]"

  Scenario: Routine informational fact does not trigger spam alert for monitored bond
    Given a clean subscription storage
    And a user with Telegram ID 55502
    And the user adds bond "RU000A107456" to their monitored bonds
    And bond "RU000A107456" has issuer INN "7708304947" and name "ЕвроТранс БО-001Р-03"
    When a material fact is detected for bond "RU000A107456":
      | Title       | Выплата купонного дохода в полном объеме |
      | PublishedAt | 2026-10-02                               |
    Then no notification message is queued for user 55502

  Scenario: Security rejection of invalid inputs in facts command
    When the user sends command "/facts ../../etc/passwd"
    Then the bot response should contain "Неверный формат запроса"
    When the user sends command "/facts 1234567890"
    Then the bot response should contain "Неверная контрольная сумма ИНН"
