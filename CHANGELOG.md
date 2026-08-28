## v0.2.0 (2026-08-27)

- add `--merged-by` and `--merged-since` filters for syncing merged PRs; `--merged-since` walks PRs by update time so it can stop early instead of crawling full repo history
- add `--filters-only` to disable auto-including PRs already in the project
- add `prs refresh` subcommand to update closed/merged PR items already on the board, with `--include-open` to also refresh a safe subset of fields on open PRs
- add new PR fields: `Merged By`, `Merged At`, `Reviewed By`, `Approved By`
- add `Changes Requested By` PR field: reviewers who requested changes in order of first request, each with their request count and total comments across those requests, e.g. `sreallymatt(×2 ✎10), katbyte(×1 ✎1)`
- `prs refresh` now diffs against current board values: only changed fields are updated, output shows `field: old -> new`, and up-to-date items are skipped
- add `CI` (`✅` passing, `❌` failing, `🕒` running) and `Mergeable` (`✅` clean, `❌` merge conflicts, `?` unknown) PR fields; both cleared when a PR closes, and `prs refresh --include-open` fetches them live for open PRs. When github hasn't computed mergeability yet the pr is re-queried with retries (as in tctest) so `?` is only stamped when it never settles
- make author/assignee/merged-by/reviewer login matching case-insensitive
- missing project fields now warn and skip by default instead of erroring; add `--strict` to error instead
- dry run now prints each field value, and sync output shows why each PR matched the filters plus a link to the PR
- retry transient network errors (http2 stream resets, connection resets, timeouts) in both GraphQL paths
- fix project field loading: handle iteration fields and fetch up to 100 fields (was 40, silently truncating)
- fix `GetItems` GraphQL query using `singleSelectOptionId` instead of `optionId`
- `SYNC_CMD` in the docker image now supports subcommand arguments (e.g. `prs refresh`)

## v0.1.0 (2026-08-03)

First tagged release of ghp-sync, a small utility to sync GitHub issues and PRs to GitHub Projects.

- sync issues and PRs from one or more repos to a GitHub Project (`issues`, `prs`) and between two projects (`project`)
- add `--sync-linked-issue-fields` to copy project field values from a PR's linked issue
- release binaries with goreleaser (linux/darwin/windows/freebsd/openbsd/solaris) on tagged releases
- publish a homebrew formula to `katbyte/homebrew-tap` on release
- modernize tooling: go 1.25, golangci-lint v2 with expanded linters, gofumpt, CI workflows (build/test/lint/depscheck/govulncheck/codeql), dependabot
- fix version reporting so `ghp-sync version` shows the real version and commit
