Feature: Webhook and Cron HTTP Endpoint Security
  As a system administrator
  I want HTTP endpoints to be hardened against unauthorized invocation and improper methods
  So that arbitrary callers cannot trigger internal operations or leak sensitive errors

  Scenario: Webhook rejects non-POST HTTP methods
    Given a bot initialized with secret token "valid-secret"
    When an HTTP "GET" request is sent to "/webhook"
    Then the response status should be 405

  Scenario: Webhook rejects unauthenticated requests when secret token is configured
    Given a bot initialized with secret token "valid-secret"
    When an HTTP "POST" request is sent to "/webhook" without secret token header
    Then the response status should be 401

  Scenario: Webhook fails closed when bot token is configured but secret token is missing
    Given a bot initialized in production mode with bot token "123:TOKEN" and no secret token
    When an HTTP "POST" request is sent to "/webhook" with body "{}"
    Then the response status should be 503

  Scenario: Webhook accepts requests with valid secret token
    Given a bot initialized with secret token "valid-secret"
    When an HTTP "POST" request is sent to "/webhook" with secret token header "valid-secret"
    Then the response status should be 200

  Scenario: Cron daily digest endpoint rejects GET method
    Given a bot initialized with secret token "valid-secret"
    When an HTTP "GET" request is sent to "/cron/daily-digest"
    Then the response status should be 405
