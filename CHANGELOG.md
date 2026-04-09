# Changelog

## [0.3.0] - 2026-04-04

### Fixed

- Frame round-trip test updated for `Frame.ID` removal (alignment with core v0.3.0)
- Replaced `time.Sleep` with deterministic polling helpers in tests

### Changed

- Adopted `testify` for test assertions

---

## [0.2.0] - 2026-03-27

### Changed

- Bumped `wspulse/hub` dependency from v0.3.0 to v0.6.0

### Added

- `wspulse/hub` version badge in README

---

## [0.1.0] - 2026-03-25

### Added

- Dual-port test server: WebSocket echo + HTTP control API
- Query-param-controlled behaviour: `?reject=1`, `?room=<id>`, `?id=<id>`, `?ignore_pings=1`
- Control endpoints: `GET /health`, `POST /kick`, `POST /shutdown`, `POST /restart`
- `READY:<ws_port>:<control_port>` startup protocol for test harness discovery
- Integration tests covering all endpoints and echo behaviour
- CI workflow

[0.3.0]: https://github.com/wspulse/testserver/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/wspulse/testserver/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/wspulse/testserver/releases/tag/v0.1.0
