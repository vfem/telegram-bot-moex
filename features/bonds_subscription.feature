Feature: Bond Subscriptions and Daily Notification Digests
  As a bond investor
  I want to subscribe to daily payment alerts and new bond announcements
  So that I stay informed about coupon inflows and market placements without manual checking

  Background:
    Given a clean subscription storage

  Scenario: User subscribes to daily payment alerts and adds bonds to watchlist
    Given a user with Telegram ID 123456789
    When the user subscribes to "daily_payments"
    And the user adds bond "SU26238RMFS4" to their monitored bonds
    And the user adds bond "RU000A107456" to their monitored bonds
    Then the user subscription state should have "daily_payments" enabled
    And the monitored bonds count for user 123456789 should be 2

  Scenario: Daily scheduler dispatches digest for due coupons
    Given user 123456789 is subscribed to "daily_payments" with bonds:
      | ISIN         |
      | SU26238RMFS4 |
    And today's date is "2026-10-15"
    And bond "SU26238RMFS4" has a coupon payment of 35.40 on "2026-10-15"
    When the daily digest scheduler runs for "2026-10-15"
    Then a notification message is queued for user 123456789
    And the notification message contains "35.40"
    And the notification message mentions "SU26238RMFS4"

  Scenario: User subscribes to new bond placement announcements
    Given a user with Telegram ID 987654321
    When the user subscribes to "new_announcements"
    And a new bond placement is detected:
      | Title       | Норильский Никель БО-001Р-07 |
      | Exchange    | MOEX                         |
      | VolumeRUB   | 50000000000                  |
      | TargetCoupon| 15.2%                        |
    When the announcement monitor runs
    Then a notification message is queued for user 987654321
    And the notification message contains "Норильский Никель"
