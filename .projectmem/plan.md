# luscious-downloader — plan

> Editable **intent** file: ideas + plans — what we *mean to do*.
> This is NOT the event log. `events.jsonl` -> `summary.md` records what
> *happened*; this file records what we *intend*. The AI reads it at
> session start and edits it directly (like `PROJECT_MAP.md`): add ideas
> and plans, check items off, move done work down to Shipped. Plans are
> never logged as events.

## Ideas
_Loose thoughts, not yet committed to._

## Active plans
_What we're working toward now. Use `- [ ]` / `- [x]` checklists._
_No active plans._

## Next
_Queued, but not started._

## Someday / maybe

## Shipped
_Move completed plans here so the top stays about the future._

- [x] Replace Python package with Go CLI. Preserve downloads, user favorites, search, list files, menu/settings, folder naming, retries, PDF/CBZ, and output paths.
- [x] Update picture-list GraphQL display input and fall back across original, video, and direct media URLs.
- [x] Replace PyPI workflow and Python install docs with Go build/install instructions.
- [x] Add shared HTTP pooling and a global in-flight request limit; raise default worker counts.
- [x] Fetch album pages and album/user batches concurrently within the `--threads` budget.
- [x] Verify with `go test ./...`, `go test -race ./...`, `go vet ./...`, and a CLI build.
- [x] Add optional session-cookie file support, scoped to first-party HTTPS requests.
- [x] Resolve relative picture URLs against the current Luscious media origin.
