# #0003 Live GraphQL validation hit intermittent DNS failure after one successful endpoint probe.

- 2026-11-11T02:47:53Z `issue`: Live GraphQL validation hit intermittent DNS failure after one successful endpoint probe. [internal/luscious/client.go:18]
- 2026-11-11T02:52:21Z `attempt`: Approved live requests validated AlbumGet, AlbumListOwnPictures, ProfileGet, AlbumList filters, CLI sorts, and pagination. [internal/luscious/client.go:18] (worked)
- 2026-11-11T02:52:31Z `fix`: Completed live checks through approved network; all current GraphQL queries and tested variables return HTTP 200 without GraphQL errors. [internal/luscious/client.go:18]
