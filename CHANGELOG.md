# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Check our main [developer changelog](https://developer.paddle.com/?utm_source=dx&utm_medium=paddle-cli) for information about changes to the Paddle Billing platform, the Paddle API, and other developer tools.

## [Unreleased]

### Added

- `account` commands to get and update the business identity of your account:
  - `account get`
  - `account patch`
- `settings` commands to get and update your account settings, with `get` and `update` for each:
  - `settings balance-currency`
  - `settings checkout-features`
  - `settings checkout-payment-methods`
  - `settings checkout-recovery`
  - `settings checkout-styles-overlay`
  - `settings checkout-url`
  - `settings currency-conversions`
  - `settings statement-descriptor`
  - `settings tax`

### Changed

- `whoami` now reports your account, authentication method, and permissions
