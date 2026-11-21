# Changelog

## Unreleased

### ⚠️ Breaking changes

- **UDP is now restricted on kernels that support it (Landlock ABI v10).**
  Without `--unrestricted-network`, UDP binding and sending are denied unless
  allowed with `--bind-udp`/`--connect-udp`. Programs that resolve names over
  DNS typically need `--connect-udp 53`.
- **Building from source now needs Go 1.26** (required by go-landlock v0.11).
- **Strict mode (no `--best-effort`) checks options against the kernel's
  Landlock ABI.** It refuses to run only if an option passed explicitly cannot
  be enforced, e.g. `--connect-udp` before ABI v10 or `--unix` before ABI v9,
  and the error names the option. v0.1.17 instead failed on any kernel
  older than ABI v9. Default restrictions the kernel cannot
  enforce are now listed in a `[landrun:warn]` message, shown at every log
  level.
- **`--unrestricted-network` no longer ignores `--bind-tcp`.** Previously,
  `--unrestricted-network` turned off every network restriction and any
  `--bind-tcp` ports were silently ignored. Now outgoing access stays
  unrestricted but TCP binding is restricted to the listed ports. Scripts that
  passed both and relied on binding to other ports must drop `--bind-tcp` (or
  list every port they need).

### Added

- Kernel compatibility in both directions: landrun targets Landlock ABI v11 and
  runs on every kernel from ABI v1 onwards, and on newer kernels. See "Kernel
  Compatibility" in the README.
- UDP network restrictions (Landlock ABI v10) with `--bind-udp` and
  `--connect-udp`. `--connect-udp` also allows binding to ephemeral port 0,
  since the kernel implicitly binds an unbound UDP socket when it first sends.
- Port ranges such as `32000-32768` in `--bind-tcp`, `--connect-tcp`,
  `--bind-udp` and `--connect-udp`.
- Bind restrictions alongside `--unrestricted-network`: `--bind-tcp`/`--bind-udp`
  restrict binding to the listed ports while outgoing access stays unrestricted.
  For UDP, ephemeral port 0 stays allowed so outgoing UDP keeps working.
  `--restrict-bind-tcp` and `--restrict-bind-udp` do the same with no ports
  allowed. UDP bind restrictions need ABI v10: without `--best-effort` they fail
  on older kernels, and with it they are skipped.
- `--unrestricted-egress` as an alias for `--unrestricted-network`.

### Changed

- go-landlock bumped from v0.9.0 to v0.11.2 (Landlock ABI v11). ABI v11 adds no
  new restrictions; go-landlock sets `no_new_privs` atomically with the sandbox.
- Expanded unit and integration test coverage.
