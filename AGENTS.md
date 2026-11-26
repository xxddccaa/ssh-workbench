# SSH Workbench development

Current scope (0.2.0-beta.1): usability-first SSH workbench with direct-open deployment and full local OpenSSH `~/.ssh/config` compatibility. Read `docs/05-current-implementation.md` for background on the original V1 baseline.

## Boundaries

- `bootstrap` composes services. HTTP handlers validate generated DTOs and call business services; do not put SQL or SSH clients in handlers.
- Keep terminal ownership, I/O, transfer behavior and browser lifecycle separate. Do not move terminal bytes into React state or convert output chunks through JSON strings.
- Credentials are ephemeral unless the owner explicitly saves them to the encrypted vault (`internal/vault`, design in `docs/03-architecture.md` §7 and §9).
- Background re-dials never carry an `Interaction`: they must not answer keyboard-interactive prompts or overwrite mismatched host keys.
- Direct-open mode is the default: any configured `--listen` address (including `127.0.0.1` and `0.0.0.0`) and HTTP / reverse-proxy `Host` / `Origin` access are allowed without mandatory bootstrap tokens, passwords or TLS. Optional `--auth` and `--remote` flags preserve password, strict Origin/CSRF and TLS enforcement when desired.
- Local OpenSSH compatibility: `~/.ssh/config` (including symlinked `Include` files and `ProxyCommand`), `~/.ssh` private keys (including symlinks), `SSH_AUTH_SOCK`, and `~/.ssh/known_hosts` are supported directly. New unknown host keys use trust-on-first-use (TOFU) or `~/.ssh/known_hosts` matches; changed/mismatched keys still prompt or fail closed.
- Reference repositories are research material, not code to copy. Do not modify the old Wails project.

## Contracts and validation

- REST source: `api/openapi.json`; stream contract: `api/terminal-protocol.md`.
- Run `make generate` after changing REST schemas; do not manually edit generated Go/TypeScript types.
- `make test` checks generation, Go race/tests/vet, TypeScript and frontend unit tests.
- `make e2e` builds and tests against loopback-only SSH/SFTP fixtures with an in-memory filesystem. Never substitute production hosts or user credentials.
- `make build` embeds the frontend in a standalone Go executable. Runtime users do not need Node.
- `make bench` (opt-in) measures resource budgets against the loopback fixture; `make dist` cross-compiles archives with checksums.
- Keep validation reports truthful: synthetic Unicode insertion does not certify an OS input method; cross-compilation does not certify Windows/Linux runtime behavior.
- Do not add placeholder controls that imply unimplemented capabilities are available.
