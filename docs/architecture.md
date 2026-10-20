# Architecture

![pagelense architecture](architecture.png)

To change the diagram, open [architecture.drawio](architecture.drawio) in draw.io.

pagelense is two Go services, web and api. web is what the user sees, and api does the real work. They talk to each other over HTTP with JSON. Both are built from the same Go module and the same Dockerfile, but they run as separate processes and separate containers.

## Services

| Service | Entry point | Port | Admin port | What it does |
|---------|-------------|------|------------|--------------|
| web     | `cmd/web`   | 8080 | 9090       | Shows the HTML pages and sends the URL to api. |
| api     | `cmd/api`   | 8081 | 9091       | Fetches the page, analyzes it and returns JSON. |

### Web

web serves three pages from Go `html/template`: the form, the result and the error page. It also serves `style.css` and `app.js`. The templates and static files are embedded in the binary with `embed`, so the image is just one file.

When the form is submitted, web calls api and renders what comes back. web doesn't know how a page is analyzed. It only knows how to call the API and show the result.

If `app.js` is running, it sends the form with `fetch` and adds an `X-Partial` header. web then renders only the content block instead of the whole layout, and `app.js` swaps it into the page. Without JavaScript the browser does a normal form post and gets the full page back. Both go through the same handler.

### API

api has one real endpoint, `POST /api/v1/analyze`, plus `GET /healthz`. It fetches the page, parses it, checks the links and returns the result as JSON. This is where all the timeouts, limits and the SSRF guard live.

### Admin ports

Each service also starts a second, small HTTP server on its admin port, with `/metrics` for Prometheus and `/debug/pprof` for profiling. It runs on its own port so it can stay private. In docker-compose only the app ports and the Prometheus UI are published, and the admin ports are only reachable inside the compose network, where Prometheus scrapes them.

**Other options:**
- Put `/metrics` and pprof on the main port. One port less, but then anyone who can reach the app can also read the metrics and start a CPU profile.
- Protect them with a token on the main port. Works, but it's one more secret to manage, and a private port is simpler.

## Packages

All code lives under `internal/`, so nothing outside this module can import it.

| Package      | What it does |
|--------------|--------------|
| `frontend`   | The web routes, the templates and the HTTP client that calls api. |
| `api`        | The API routes, reading the JSON request and the JSON error format. |
| `analyzer`   | Checks the URL, then reads the HTML and finds the version, title, headings, links and login form. |
| `fetcher`    | Downloads the page with timeouts, a body size limit and a redirect limit. Blocks private addresses. |
| `linkcheck`  | Checks links with a pool of workers, a per-link timeout and a total time budget. |
| `middleware` | Request ID, request logs and panic recovery, used by both services. |
| `metrics`    | Prometheus metrics, the HTTP middleware and wrappers that time the analyzer and the link checker. |
| `admin`      | The admin server with `/metrics` and `/debug/pprof`. |
| `config`     | Reads settings from environment variables and checks them. |
| `serve`      | Starts the servers and shuts them down cleanly on SIGTERM. |

### How the packages fit together

The packages depend on each other in one direction:

```
cmd/api  → api → analyzer → fetcher
                          → linkcheck
cmd/web  → frontend → (HTTP) → api
```

`main.go` in each service is the only place where things are wired together. It reads the config, creates the fetcher, the link checker and the analyzer, wraps them with metrics, and passes them into the HTTP handler. No package creates its own dependencies, and there are no globals apart from the default logger.

Each package defines a small interface for what it needs, in the package that uses it. For example `api` only needs something with an `Analyze(ctx, url)` method, and `analyzer` only needs a `Fetcher` and a `LinkChecker`. This makes testing easy, because a test can pass in a fake that returns whatever the test needs, with no network.

It also means some nice reuse:

- **The api client has the same shape as the analyzer.** `frontend` needs an `Analyzer` too, and the HTTP client that calls api implements that same `Analyze` method. So the web handler doesn't know or care that the analysis happens in another service. If you ever wanted one binary, you could pass the real analyzer in instead of the client.
- **Metrics wrap things instead of being mixed into them.** `metrics.InstrumentAnalyzer` takes an analyzer and returns one that also records how long each call took. The same goes for the link checker and the backend client. The analyzer code has no Prometheus code in it at all.

**Other options:**
- Put everything in one package. Fewer files, but hard to test parts on their own.
- Use a dependency injection framework like `wire` or `fx`. Useful in big apps, but here `main.go` is around 60 lines and easy to read as it is.
- Call Prometheus directly inside the analyzer. Less code, but it ties the analyzer to Prometheus, and the tests have to deal with global metrics.

## Request flow

This is what happens when a user analyzes a page:

1. The browser posts the URL to `POST /analyze` on web.
2. The web middleware gives the request an ID (or reuses the `X-Request-ID` header if one came in) and stores it in the request context.
3. The web handler calls the api client. The client sends `{"url": "..."}` to `POST /api/v1/analyze` with the same `X-Request-ID`, so both services log the same ID.
4. api reads the JSON body (max 64 KB) and calls the analyzer.
5. The analyzer checks the URL with `net/url`. It must be `http` or `https` and have a host, otherwise the request stops here with a `400`.
6. The fetcher downloads the page. Before every connection, the dialer checks the resolved IP and refuses private, loopback and link-local addresses. It follows up to 10 redirects, reads at most 5 MB, and only accepts HTML.
7. The analyzer parses the HTML once with `golang.org/x/net/html` and walks the tree a single time, collecting the doctype, the first `<title>`, heading counts, `<base href>`, all `<a href>` values and login fields.
8. The links are resolved to full URLs and split into internal and external.
9. The link checker checks up to 100 unique links with 10 workers. Each link gets `HEAD` first and `GET` if `HEAD` fails, with 5 seconds per link and 10 seconds for all of them together.
10. api returns the result as JSON.
11. web renders the result page, or only its content block if `app.js` asked for it.

### When something goes wrong

Every error in api becomes the same JSON shape:

```json
{"error": {"status": 404, "message": "example.com returned 404 Not Found"}}
```

The message is always safe to show to a user. The real error, with details like the IP or the Go error text, is only written to the log for that request ID.

| What failed | Status from api |
|-------------|-----------------|
| Bad JSON or a bad URL | `400` |
| Request body over 64 KB | `413` |
| The page returned an error status | The same status, like `404` or `503` |
| The page isn't HTML | `415` |
| The host resolves to a private address | `403` |
| The page took too long | `504` |
| DNS failed, connection refused, too many redirects or page over 5 MB | `502` |
| A bug or a panic | `500` with a generic message |

web turns the API error into its error page with the same status and message. If api itself can't be reached, web shows `502`, and if api takes too long, `504`. If api sends back something that isn't a proper error body, web shows `502` instead of passing on whatever it got.

**Other options:**
- Pass the Go error text straight to the user. Easier to debug, but it can leak internal details like IP addresses or file paths.
- Use a standard format like RFC 9457 (`application/problem+json`). A good choice for a public API, but more fields than this project needs.

## Timeouts

Every network call has a limit, so a slow site can't keep a request open forever.

| Setting | Default | What it limits |
|---------|---------|----------------|
| `FETCH_TIMEOUT` | 10s | Downloading the page, including redirects and reading the body |
| `LINK_TIMEOUT` | 5s | One link check |
| `LINK_BUDGET` | 10s | All link checks for one analysis |
| `API_TIMEOUT` | 30s | web waiting for api; must be longer than `FETCH_TIMEOUT` + `LINK_BUDGET` |
| `SHUTDOWN_TIMEOUT` | 15s | How long in-flight requests get to finish on shutdown |

The HTTP servers also have read timeouts (5s for headers, 10s for the body) so slow clients can't hold connections open. api has no write timeout on purpose, because an analysis can legally take up to `FETCH_TIMEOUT` + `LINK_BUDGET`, and a write timeout would cut it off. The request context is passed all the way down, so if the user closes the tab, the fetch and the link checks stop too.

## Shutdown

Both services listen for SIGTERM and Ctrl+C. When one arrives, `serve.Run` stops accepting new connections on both the app and admin servers and gives in-flight requests up to `SHUTDOWN_TIMEOUT` to finish. If one of the servers fails to start (for example the port is taken), the other one is shut down too, so the process doesn't half-run.

## Logging and metrics

Every request gets one JSON log line from `log/slog` with the method, path, status, duration and request ID. Errors that were recorded during the request are added to that line, so the log shows the real cause behind a short user message.

Prometheus metrics include request counts and durations per route, how long analyses take, link check results, and backend errors as seen from web. See [observability.md](observability.md#observability) for the full list and screenshots.

## Build and run

The Dockerfile has one build stage and two final targets, `api` and `web`. Both are built with `CGO_ENABLED=0` into static binaries and copied into `gcr.io/distroless/static`, running as a non-root user. The images have no shell or package manager, which keeps them small and leaves very little to attack. The base images are pinned by digest so builds don't change by surprise.

`docker-compose.yml` runs api, web and Prometheus. web finds api at `http://api:8081` on the compose network.

**Other options:**
- An Alpine or Debian base image. Lets you `exec` into the container with a shell, which is handy for debugging, but brings in a lot of extra packages.
- `scratch`. Even smaller than distroless, but you have to add CA certificates and a non-root user yourself.
- Two Dockerfiles. Works, but most of the build is the same, so one file with two targets is less to keep in sync.

## Libraries

| Library | Why | Other options |
|---------|-----|---------------|
| Gin | Routing, JSON binding and middleware with little code, and it's widely used | The standard `net/http` mux (Go 1.22+ has method and path patterns), Echo, chi |
| `golang.org/x/net/html` | The HTML5 parser from the Go team. It handles broken HTML the same way browsers do | goquery (jQuery-style selectors on top of it, nicer to write but one more dependency), regex (breaks on real pages) |
| `log/slog` | In the standard library, JSON output, no dependency | zap, zerolog (a bit faster, but not needed here) |
| Prometheus client | The standard for Go metrics, and Prometheus is easy to run in compose | OpenTelemetry (covers metrics and traces, but much more setup) |
