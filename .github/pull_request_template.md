## Summary
<!-- What does this PR do, in 1-2 sentences? -->

## Related issue
<!-- Link the issue this PR closes/relates to -->
Closes #

## Type of change
- [ ] Bug fix
- [ ] New feature
- [ ] Refactor / chore
- [ ] Documentation
- [ ] Research / investigation

## Affected area
- [ ] Frontend (React)
- [ ] Desktop shell (Tauri)
- [ ] Mobile (Tauri)
- [ ] Backend (Go)
- [ ] Other

## Changes
<!-- Bullet list of key changes -->
-
-

## Validation
<!-- Check every affected area and replace placeholders with the exact commands you ran before opening this PR. Remove lines that are not relevant. -->

### Frontend apps (`client/mobile/` and `client/desktop/`)

Install the shared workspace dependencies once from `client/`. These root
commands run the corresponding script in both frontend apps:

- [ ] `corepack enable`
- [ ] `yarn install --immutable`
- [ ] `yarn format`
- [ ] `yarn lint`
- [ ] `yarn typecheck`
- [ ] `yarn test`
- [ ] `yarn build`

If a change only affects one app, use the matching workspace command from
`client/`, for example `yarn workspace anomaly-desktop test`.

### Tauri / Rust (`client/mobile/src-tauri/` and `client/desktop/src-tauri/`)

Run these commands in each affected app's `src-tauri/` directory:

- [ ] `cargo fmt --check`
- [ ] `cargo clippy --all-targets -- -D warnings`
- [ ] `cargo test`

### Go backend (`server/`)
- [ ] `gofumpt -l .`
- [ ] `golangci-lint run`
- [ ] `go test ./...`

### Python service (`python-service/`)
- [ ] `ruff format --check .`
- [ ] `ruff check .`
- [ ] `pytest`

## Screenshots(If UI changed)
<!-- If UI-related, add before/after screenshots or a short recording -->

## Checklist
- [ ] Code builds and runs locally
- [ ] Ran all relevant automated tests before opening this PR and recorded the exact commands in `Validation`
- [ ] Tested on affected platform(s)
- [ ] Covered every impacted area (`client/mobile/`, `client/desktop/`, `server/`, `python-service/`) or marked it not applicable
- [ ] No breaking changes to existing features
- [ ] Docs/comments updated if needed

## Additional notes
<!-- Anything reviewer should know -->
