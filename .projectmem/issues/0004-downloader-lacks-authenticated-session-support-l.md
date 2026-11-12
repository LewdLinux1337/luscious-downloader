# #0004 Downloader lacks authenticated-session support; Luscious login and cookie requirements need verification.

- 2026-11-11T02:54:49Z `issue`: Downloader lacks authenticated-session support; Luscious login and cookie requirements need verification. [internal/luscious/client.go:143]
- 2026-11-11T03:04:53Z `attempt`: Added cookie-file input and first-party HTTPS cookie scoping; auth parsing and transport tests pass. [cmd/lsd/main.go] (worked)
- 2026-11-11T03:05:13Z `fix`: Added optional session-cookie file input; cookie values stay out of settings and logs and reach only HTTPS requests to members/www.luscious.net. [cmd/lsd/main.go]
