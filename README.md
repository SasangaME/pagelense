# pagelense

Give it a URL and it tells you what's on the page: HTML version, title, heading counts, internal and external links (and how many are broken), and whether there's a login form.

It's two small Go services:

- **web** on http://localhost:8080 – the HTML pages
- **api** on http://localhost:8081 – the JSON API that does the work

More detail in [docs/architecture.md](docs/architecture.md) (diagram and request flow) and [docs/api.md](docs/api.md) (endpoints and errors).

## Tech stack

- **Backend:** Go 1.27, Gin, `golang.org/x/net/html` for parsing, `log/slog` for JSON logs
- **Frontend:** Go `html/template`, plain CSS, one small JS file
- **Ops:** Docker (distroless images, non-root), docker-compose, Prometheus, pprof, GitHub Actions, golangci-lint, Dependabot

## Running it

You need Docker, or Go 1.27+ if you want to run it without Docker.

```sh
make compose-up      # web :8080, api :8081, Prometheus :9090
make compose-down
```

Without Docker, run each service in its own terminal:

```sh
cp .env.example .env
make run-api
make run-web
```

All settings are environment variables. `.env.example` lists them with comments.

```sh
make test    # go test -race
make cover   # fails under 70%
make lint
```

## How it works

Type a URL into the form. web sends it to `POST /api/v1/analyze` on api. api fetches the page, parses it, checks the links concurrently and returns JSON, and web renders the result.

- **URL validation:** `net/url`, no regex. Must be `http://` or `https://` with a host.
- **Errors:** every API error is `{"error": {"status", "message"}}`. The message is safe to show and the real cause goes to the log. web shows the same status and message on its error page.
- **Logging:** one JSON line per request with method, path, status, duration and request ID. The `X-Request-ID` is passed from web to api, so you can follow one request through both logs.
- **Metrics:** each service has an admin port with `/metrics` and `/debug/pprof`. compose doesn't publish it, and Prometheus scrapes it.
- **Shutdown:** on SIGTERM the servers stop taking new requests and finish what's in flight.

## Decisions

- **Separate frontend.** web calls api over HTTP, so the API stands on its own and each side can scale separately.
- **Private addresses are blocked.** Users can't make the server fetch internal or cloud metadata URLs. Because of this, `localhost` pages can't be analyzed.
- **Link checks have limits.** At most 100 links, 10 at a time, in 10 seconds, so big pages don't hang.
- **The page's status is passed through.** If the page returns 404, the API returns 404 too.
- **Works without JavaScript.** `app.js` only avoids a full page reload.
- **No database, no cloud.** Nothing needs storing, and the task asks for Docker or a Makefile.
- **Few comments.** Exported names and packages have doc comments because the linter (`revive`) requires them. Other comments only explain why code does something unusual.

### Assumptions

- The URL must include `http://` or `https://`.
- Internal means the exact same hostname, so `www.example.com` is external to `example.com`.
- Only `<a href>` links count, and duplicates are counted each time. `mailto:`, `javascript:` and `#anchor` links are skipped.
- Inaccessible means the link didn't respond or returned 4xx/5xx. It's counted out of the checked links, not all links.
- An unrecognized or missing doctype is reported as `Unknown`.
- A login form means there's a password input or an input with `autocomplete="username"` (for sign-ins that ask for the password on the next step). Field names and button text aren't used.
- The HTML isn't run, so anything rendered by JavaScript (including login forms) isn't seen.

## Challenges

- **Slow link checks.** Some hosts hang on every link. A worker pool plus a per-link timeout and a total time budget keep each analysis under about 20 seconds.
- **Getting SSRF right.** Checking the hostname before the request isn't enough, because DNS can change or a redirect can point inside. Doing the check in the dialer's `Control` hook covers all of that.
- **Login detection.** Many sign-in pages ask for the username first and the password later, so a check for a password field alone missed them. Looking for `autocomplete="username"` as well caught those pages.

## Possible improvements

- Cache results for a few minutes, so analyzing the same URL twice doesn't fetch it again
- Respect `robots.txt` and rate-limit link checks per host
- Run large analyses as a background job that the page polls
- Deploy to AWS: both services on ECS Fargate behind one ALB, images in ECR, and CI deploying through GitHub OIDC with no stored keys. ECS sends SIGTERM before it stops a task, so the existing graceful shutdown already works there.
- An integration test that runs web → api → a local test site
