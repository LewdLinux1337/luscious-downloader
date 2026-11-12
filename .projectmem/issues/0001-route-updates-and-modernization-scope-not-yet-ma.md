# #0001 Route updates and modernization scope not yet mapped for proposed Go rewrite

- 2026-11-11T01:57:17Z `issue`: Route updates and modernization scope not yet mapped for proposed Go rewrite
- 2026-11-11T02:15:09Z `attempt`: Implemented Go CLI and changed picture list display to date_newest with URL fallback; go build ./... passes [internal/luscious/client.go] (partial)
- 2026-11-11T02:21:42Z `attempt`: Hardened URL parsing, GraphQL null IDs, downloads, and conversion cleanup; go build, go vet, and diff check pass [cmd/lsd/main.go] (worked)
- 2026-11-11T02:24:25Z `attempt`: Preserved read-only CLI --list and comma-separated list entries; Go build and vet pass [cmd/lsd/main.go] (worked)
- 2026-11-11T02:24:31Z `fix`: Replaced Python with Go CLI, updated picture-list GraphQL display input, added media URL fallbacks, and preserved the existing CLI/menu features; build and vet pass [internal/luscious/client.go]
- 2026-11-11T02:26:14Z `attempt`: Restored menu error logging to logs.log; Go build, vet, and diff checks pass [cmd/lsd/main.go] (worked)
