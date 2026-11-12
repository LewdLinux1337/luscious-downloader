# Luscious Downloader

Go CLI for downloading Luscious albums, user albums, and favorites.

## Install

Install Go 1.22 or newer, then run:

```sh
go install github.com/LewdLinux1337/luscious-downloader/cmd/lsd@latest
```

Or build from a checkout:

```sh
go build -o lsd ./cmd/lsd
```

## Use

```sh
lsd --album https://www.luscious.net/albums/album-name_275719/
lsd --album 275719,292887 --output ./albums
lsd --user https://www.luscious.net/users/668124/ --favorites
lsd --search yuri --page 2 --max-page 5
lsd --search yuri --download --pdf --cbz
lsd --list
lsd --menu
```

Run `lsd --help` for all flags. Running `lsd` without arguments opens the interactive menu.

The CLI accepts album and user IDs or URLs from `www.luscious.net` and `members.luscious.net`. Album folders default to `[%i]%t`; `%i`, `%t`, `%a`, `%p`, and `%g` mean ID, title, author, picture count, and GIF count.

## Authenticated sessions

Use a session cookie exported from your own signed-in browser when the site requires authentication. Save the raw `Cookie` header value on one line in a local file, then pass `--cookie-file` or set `LUSCIOUS_COOKIE_FILE` for menu use:

```sh
chmod 600 luscious-cookie.txt
lsd --cookie-file luscious-cookie.txt --album 275719
LUSCIOUS_COOKIE_FILE=./luscious-cookie.txt lsd --menu
```

The cookie is sent only over HTTPS to `members.luscious.net` and `www.luscious.net`. The CLI does not save it in `config.json` or logs. Do not commit or share the cookie file. Site permissions still apply.

Downloads use a shared HTTP connection pool and one global in-flight request limit. Album, search, and user pages run concurrently when page totals are available; multiple albums and users download concurrently while preserving output order. `--threads` sets network worker limit (default 16–64 based on CPU, maximum 128). Existing non-empty files are skipped. Failed or empty responses do not replace completed files.

`--pdf` creates one PDF per album from JPG and PNG files. `--cbz` creates a ZIP-based CBZ. `--rm-origin-dir` removes album folders only after successful conversion.

The interactive menu creates `config.json`, `list.txt`, and `list_completed.txt` in the current directory. The menu's default output folder is `./albums/`; direct CLI downloads default to the current directory.
