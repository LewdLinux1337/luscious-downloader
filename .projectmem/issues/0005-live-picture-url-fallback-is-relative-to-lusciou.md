# #0005 Live picture URL fallback is relative to luscious.net; downloader requires absolute URLs.

- 2026-11-11T03:00:23Z `issue`: Live picture URL fallback is relative to luscious.net; downloader requires absolute URLs. [internal/luscious/client.go:289]
- 2026-11-11T03:05:02Z `attempt`: Resolved relative GraphQL picture URLs against the current first-party media origin; URL tests pass. [internal/luscious/client.go] (worked)
- 2026-11-11T03:05:22Z `fix`: Relative picture URLs now resolve against https://www.luscious.net/ before downloads. [internal/luscious/client.go]
