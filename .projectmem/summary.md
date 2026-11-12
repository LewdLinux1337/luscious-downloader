# projectmem - luscious-downloader

_Last updated: 2026-11-11_

## Project purpose
CLI tool for downloading [Luscious](https://luscious.net) Albums

## Recent issues
- [DONE] #0005 Live picture URL fallback is relative to luscious.net; downloader requires absolute URLs. [internal/luscious/client.go:289] -> Relative picture URLs now resolve against https://www.luscious.net/ before downloads. [internal/luscious/client.go] (fixed)
- [DONE] #0004 Downloader lacks authenticated-session support; Luscious login and cookie requirements need verification. [internal/luscious/client.go:143] -> Added optional session-cookie file input; cookie values stay out of settings and logs and reach only HTTPS requests to members/www.luscious.net. [cmd/lsd/main.go] (fixed)
- [DONE] #0003 Live GraphQL validation hit intermittent DNS failure after one successful endpoint probe. [internal/luscious/client.go:18] -> Completed live checks through approved network; all current GraphQL queries and tested variables return HTTP 200 without GraphQL errors. [internal/luscious/client.go:18] (fixed)
- [DONE] #0002 Network throughput may be limited by sequential album pagination and default HTTP connection pooling -> Shared pooling, bounded concurrent pagination and album batches; tests and race detector pass. [internal/netx/client.go] (fixed)
- [DONE] #0001 Route updates and modernization scope not yet mapped for proposed Go rewrite -> Replaced Python with Go CLI, updated picture-list GraphQL display input, added media URL fallbacks, and preserved the existing CLI/menu features; build and vet pass [internal/luscious/client.go] (fixed)
  - Partial attempt: Implemented Go CLI and changed picture list display to date_newest with URL fallback; go build ./... passes [internal/luscious/client.go]

## Decisions
- Replace Python downloader with Go and preserve full CLI/menu, search, user favorites, list, PDF, CBZ, and settings behavior [cmd/lsd]
- Use one pooled standard-library HTTP client with a shared in-flight request limit; avoid dependency for worker-pool concurrency. [internal/netx/client.go]
- Accept user-supplied session cookies from a local file; forward only to HTTPS members/www.luscious.net, never persist credentials in config. [internal/netx/client.go]

## Notes
- re-added exceptions
- gotcha: Nov 7, 2026 Luscious GraphQL rejects outdated PictureListInput.display with HTTP 400; album query needs current enum [luscious_dl/querys.py]

## Key files
- `.github/workflows/python-publish.yml`
- `.gitignore`
- `LICENSE`
- `MANIFEST.in`
- `README.md`
- `luscious_dl/__init__.py`
- `luscious_dl/album.py`
- `luscious_dl/command_line.py`
- `luscious_dl/downloader.py`
- `luscious_dl/exceptions.py`
- `luscious_dl/logger.py`
- `luscious_dl/menu.py`
- `luscious_dl/parser.py`
- `luscious_dl/querys.py`
- `luscious_dl/start.py`
- `luscious_dl/user.py`
- `luscious_dl/utils.py`
- `requirements.txt`
- `setup.py`
- `PictureListInput.display`

## Open questions
- None logged yet.
