# pagelense

pagelense is a web app that analyzes a web page from its URL. For each page it shows:

- HTML version
- Page title
- Number of headings by level (h1–h6)
- Number of internal and external links
- Number of inaccessible links
- Whether the page has a login form

It's two small Go services:

- **web** on http://localhost:8080 – the HTML pages
- **api** on http://localhost:8081 – the JSON API that does the work

Each service also has an admin port with `/metrics` and `/debug/pprof`: 9090 for web and 9091 for api. With `make run-web` and `make run-api` they're on localhost. With compose they stay inside the compose network, where Prometheus scrapes them, and http://localhost:9090 is the Prometheus UI.

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

## Architecture

![pagelense architecture](docs/architecture.png)

The services, packages and request flow are in **[docs/architecture.md](docs/architecture.md#architecture)**.

## API

```sh
curl -X POST http://localhost:8081/api/v1/analyze \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com"}'
```

Endpoints, the response format and every error status are in **[docs/api.md](docs/api.md#api)**.

## Testing

- All tests pass with `-race`
- 90.7% coverage (`make cover` fails under 70%)
- 0 lint issues

The per-package results, manual test cases with screenshots and API tests with Bruno are in **[docs/testing.md](docs/testing.md#testing)**.

## Observability

- JSON request logs with a request ID that follows each request from web to api
- Prometheus metrics for requests, analysis time, link checks and backend errors
- pprof on each service's admin port

The logs, metrics, Prometheus setup and screenshots are in **[docs/observability.md](docs/observability.md#observability)**.

## How it works

Type a URL into the form. web sends it to `POST /api/v1/analyze` on api. api fetches the page, parses it, checks the links concurrently and returns JSON, and web renders the result.

- **URL validation:** `net/url`, no regex. Must be `http://` or `https://` with a host.
- **Errors:** every API error is `{"error": {"status", "message"}}`. The message is safe to show and the real cause goes to the log. web shows the same status and message on its error page.
- **Logging:** one JSON line per request with method, path, status, duration and request ID. The `X-Request-ID` is passed from web to api, so you can follow one request through both logs.
- **Metrics:** each service has an admin port with `/metrics` and `/debug/pprof`. compose doesn't publish it, and Prometheus scrapes it.
- **Shutdown:** on SIGTERM the servers stop taking new requests and finish what's in flight.

## Decisions

### Separate frontend and API

web and api are two services. web only renders HTML and calls api over HTTP, and api does all the fetching and parsing. This way the API can be used on its own (curl, Bruno, another client), and each side can be scaled or deployed separately. It also keeps the code honest, because the frontend can't reach into the analyzer and has to go through the same API as everyone else.

The cost is one more service to run and one extra network hop per request, and the request ID has to be passed between them so the logs still line up.

**Other options:**
- One Go binary that serves both the pages and the API. Simpler to run, but the two get mixed together easily.
- A single page app (React, Vue) talking to the API. Nicer for a big UI, but it needs a Node build step and is too much for one form and one result page.

### Private addresses are blocked

The server fetches whatever URL the user types in, so without a guard someone could make it fetch `http://169.254.169.254/` (cloud metadata) or a service on the internal network. To stop that, the fetcher's dialer refuses to connect to loopback, private, link-local and unspecified IPs. The check runs in the dialer's `Control` hook, after DNS has resolved, so it also catches redirects and hostnames that resolve to an internal IP.

The downside is that `localhost` pages can't be analyzed, which is a bit annoying while testing.

**Other options:**
- Check the hostname or IP once before making the request. Easy to write, but a redirect or a DNS change after the check gets around it.
- An allowlist of domains. Very safe, but then the tool can't analyze any page you want.
- A flag to allow private addresses in local development. Handy, but it's one more setting that could be left on by mistake.

### Link checks have limits

Checking every link one by one on a big page can take minutes, and some hosts never answer. So each analysis checks at most 100 unique links, 10 at a time, with 5 seconds per link and 10 seconds for all of them together. Each link is tried with `HEAD` first and falls back to `GET` if the server doesn't like `HEAD`. Links that don't finish in time just aren't counted as checked. All of these numbers can be changed with environment variables.

This means on a very big page the inaccessible count is based on a sample, not on every link.

**Other options:**
- Check every link with no limit. Accurate, but one bad page can hang a request for a long time.
- Don't check links in the request at all, and run it as a background job that the page polls. Better for huge pages, but needs a queue and some storage (see Possible improvements).
- Only use `GET`. Fewer edge cases, but downloads a lot more data.

### The page's status is passed through

If the target page returns an error status, like 404 or 503, the API returns that same status with a short message, instead of `200` with an error inside or a generic `502`. That way the user sees exactly what went wrong with the page, and API clients can just look at the status code.

The catch is that a 404 from the API could mean "the page wasn't found" rather than "the endpoint doesn't exist", so the error message names the host, like `example.com returned 404 Not Found`.

**Other options:**
- Always return `502 Bad Gateway` when the target fails, and put the real status in the body. More correct in HTTP terms, but less clear for the user.
- Return `200` and analyze the error page anyway. Some error pages have content, but the results would usually be confusing.

### Works without JavaScript

The form is a normal HTML form that posts to web, and the result is rendered on the server with `html/template`. `app.js` (about 30 lines) only takes over the submit so the page doesn't fully reload. If JavaScript is off or fails to load, everything still works.

**Other options:**
- Do everything in the browser with `fetch` and build the result with JS. Feels smoother, but breaks without JS and means writing the rendering twice.
- htmx or a similar library. Nice, but an extra dependency for one form.

### No database, no cloud

Every analysis is done fresh and nothing has to be kept, so there's no database. The task also asks for Docker or a Makefile, so everything runs locally with `make` or `docker compose`, and there's no cloud setup to sign up for.

**Other options:**
- Redis or Postgres to cache results or keep a history of analyzed URLs. Useful later, but not needed now.
- Deploy to AWS from the start. There's a plan for this under Possible improvements, but it isn't needed to run or review the project.

### Few comments

Exported names and packages have doc comments because the linter (`revive`) requires them. Other comments are only there to explain why code does something unusual, like why the SSRF check is in the dialer. The idea is that clear names and small functions should explain the "what", and comments should explain the "why".

**Other options:**
- Comment every function. More text to read, and it tends to get out of date.
- No comments at all apart from what the linter needs. Shorter, but the tricky parts would be hard to follow.

## Assumptions

The task leaves some things open, so these are the rules I went with. Each one is a choice, and some could easily go the other way.

### The URL must include `http://` or `https://`

`example.com` on its own is rejected with a clear message. The URL is checked with `net/url`, not a regex, and it must have an `http` or `https` scheme and a host.

**Other options:**
- Add `https://` when there's no scheme. Friendlier, but then the user doesn't know exactly what was fetched, and some sites only work on `http`.
- Try `https://` first and fall back to `http://`. Nice, but it hides problems and doubles the work when a site is down.

### Internal means the exact same hostname

A link is internal only if its hostname matches the page's hostname exactly (ignoring case). So on `example.com`, a link to `www.example.com` or `blog.example.com` counts as external. Relative links are resolved against the page URL, or against `<base href>` if the page has one. The port and scheme don't matter, so `http://example.com` and `https://example.com` are both internal.

**Other options:**
- Count any subdomain of the same site as internal. Closer to what people usually mean, but you need the public suffix list to do it properly, otherwise `a.co.uk` and `b.co.uk` would look like the same site.
- Treat `www.` as the same as the bare domain. A common special case, but still a guess.

### Only `<a href>` links count, and duplicates are counted every time

Links in `<link>`, `<script>`, `<img>` and so on aren't counted, because the task is about links on the page, not resources. If the same link appears five times, it counts five times, because that's what's on the page. `mailto:`, `tel:`, `javascript:` and other non-http links are skipped, and so are links that are only `#anchor`. A link like `/page#section` is counted, without the `#section`.

**Other options:**
- Count unique links only. Gives a cleaner number, but hides how the page is really built.
- Also count `<area href>` and links inside iframes. More complete, but rare and adds edge cases.

### Inaccessible means no response or a 4xx/5xx status

A link is inaccessible if the request fails, times out, or returns 400 or above. Redirects are followed, and only the final status counts. The count is out of the links that were actually checked (at most 100 unique ones), not out of all links on the page, and the result shows how many were checked.

**Other options:**
- Treat only 404 and 410 as broken. Some sites return 403 or 429 to bots even when the link works fine for people, so this would give fewer false alarms, but it also misses real problems like 500s.
- Scale the result up to guess the total for big pages. Gives a number for every link, but it's an estimate, not a fact.

### An unrecognized or missing doctype is `Unknown`

`<!DOCTYPE html>` is HTML5. The common HTML 4.01 and XHTML 1.0/1.1 doctypes are matched by their public ID. Anything else, or no doctype at all, is reported as `Unknown`.

**Other options:**
- Report a missing doctype as HTML5, since that's how browsers treat most pages now. But browsers actually use quirks mode then, so that would be wrong.
- Guess the version from the tags that are used. Too unreliable to be useful.

### What counts as a login form

The page has a login form if it has an `<input type="password">` or an input with `autocomplete="username"`. The second rule is for sign-in pages that ask for the username first and the password on the next step (Google, Microsoft and many others work like this). Field names and button text like "Log in" aren't used, because they change with language and are easy to get wrong.

A sign-up form with a password field will also count as a login form. I think that's fine, since both are a place where users sign in with a password.

**Other options:**
- Only look for a password field. Simple, but misses the two-step pages.
- Look at words like "login", "sign in" or "username" in names, ids and buttons. Catches more pages, but gives false positives and only works in English.
- Require the inputs to be inside a `<form>` tag. Stricter, but many modern pages don't use a form tag at all.

### JavaScript isn't run

The HTML is parsed as it comes from the server. Anything that's added by JavaScript later, including login forms, links and headings in single page apps, isn't seen.

**Other options:**
- Render the page in a headless browser (Chrome with `chromedp` or Playwright). Sees what a real user sees, but it's much slower, uses a lot of memory, and running other people's JavaScript on the server is a security risk in itself.
- Use headless rendering only when the page looks empty. A good middle ground, but it means having two code paths.

## Challenges

### Slow link checks

My first version checked links one after another, and some pages took over a minute. Making it concurrent fixed most pages, but some hosts just hang on every request, so a single bad host could still hold up the whole result.

What worked was a fixed pool of 10 workers, a 5 second timeout per link, and a 10 second budget for all checks in one analysis. When the budget runs out, the links that are still waiting are dropped and aren't counted as checked. With the page fetch on top of that, an analysis takes at most about 20 seconds. Using `HEAD` first and falling back to `GET` also helped, because `HEAD` doesn't download the body.

**Other ways I could have done it:**
- Start one goroutine per link with no limit. Fast, but a page with 2,000 links would open 2,000 connections at once, and some sites would block it.
- Return the page results right away and check links in the background. The best experience, but needs polling or websockets and somewhere to keep the state.

### Getting SSRF right

The first idea was to look up the hostname, check the IP, and then make the request. That has two gaps. The DNS answer can change between the check and the real request (DNS rebinding), and a public URL can redirect to `http://127.0.0.1` or an internal address.

Moving the check into the dialer's `Control` hook fixed both. That hook runs right before every TCP connection, with the IP that's actually being used, so it covers the first request, every redirect, and every link check. It took some time to write tests for it, since the tests themselves use `httptest` servers on `127.0.0.1`, which the guard blocks. The tests use a fetcher with the guard turned off for normal cases and a separate set of tests for the guard.

**Other ways I could have done it:**
- Check the IP once before the request and turn off redirects. Closes the redirect gap, but not the DNS one, and lots of real sites need redirects.
- Send all requests through an egress proxy that blocks internal ranges. Very solid, and a good idea in production, but too heavy for this project.

### Login detection

Checking for a password field worked on simple pages, but missed a lot of big sites, because they ask for the email or username first and show the password field on the next page. I tried matching words like "login" and "email" in field names, but that gave false positives on newsletter sign-ups and search boxes.

`autocomplete="username"` turned out to be a good signal. Browsers and password managers use it, so sites that care about sign-in usually set it, and it means the same thing in every language.

**Other ways I could have done it:**
- Follow the form and look at the next step. Not possible without filling in a real username.
- A scoring system using several weak signals. Could be more accurate, but hard to explain and test.

## Possible improvements

### Cache results

Keep each result for a few minutes, keyed by the normalized URL. If someone analyzes the same URL again, or a lot of people try the same popular page, it doesn't have to fetch the page and check 100 links again. An in-memory LRU cache is enough for one instance. With more than one instance, Redis would let them share the cache.

### Be polite to other sites

Respect `robots.txt`, send a clear `User-Agent` with a contact link, and rate-limit link checks per host, so one page with 100 links to the same site doesn't hit that site 10 times at once. This matters more once the tool is public, because it can be used to send traffic to someone else's site.

### Rate-limit the API

Right now anyone can call `/api/v1/analyze` as often as they want, and each call can make up to 101 outgoing requests. A per-IP rate limit (a token bucket in a Gin middleware, or at the load balancer) would stop one user from using up all the capacity.

### Background jobs for big pages

For pages with many links, return a job ID right away, check links in the background, and let the page poll for the result (or use server-sent events). This would also allow checking all links instead of only the first 100. It needs a queue and somewhere to store job state, for example Redis or SQS with DynamoDB.

### An end-to-end test

Add a test that starts web, api and a small local test site, submits the form, and checks the rendered result. The unit tests already cover each package, but this would catch problems in the wiring between the two services, like a wrong `API_URL` or a missing request ID header.

### Deploy to AWS

This is how I'd run it on AWS:

- **Compute:** both services as separate ECS Fargate services, so they scale on their own. No servers to patch.
- **Load balancer:** one Application Load Balancer with HTTPS (a certificate from ACM). `/api/*` goes to api and everything else goes to web. web calls api through an internal address, so that traffic doesn't leave the VPC.
- **Network:** tasks run in private subnets. api needs to reach the internet to fetch pages, so it goes out through a NAT gateway. The SSRF guard still matters here, because it already blocks the VPC's private ranges and the ECS metadata address (`169.254.170.2`).
- **Images:** pushed to ECR, with scan on push turned on.
- **Config and secrets:** environment variables in the task definition, and anything secret in SSM Parameter Store or Secrets Manager.
- **Logs and metrics:** the JSON logs go to CloudWatch Logs as they are. For metrics, Amazon Managed Service for Prometheus can scrape the admin port, or a CloudWatch agent sidecar can. The admin port stays closed to the internet.
- **Scaling:** ECS auto scaling on CPU or on request count per target.
- **Shutdown:** ECS sends SIGTERM and waits before it kills a task, and the ALB stops sending new requests, so the existing graceful shutdown already works there. The ECS stop timeout should be a bit longer than `SHUTDOWN_TIMEOUT`.
- **Infrastructure as code:** all of the above in Terraform (or AWS CDK), kept in the repo, so the setup can be reviewed and recreated.

**Other options:** App Runner is simpler and handles the load balancer and scaling, but gives less control over the network. Lambda would work for api, but long link checks and the 15 minute limit make it a poor fit for the background job version. EKS works too, but it's too much for two small services.

### CI/CD

**CI on every pull request** (GitHub Actions):

1. `go vet` and `golangci-lint`
2. `go test -race` with the 70% coverage check (`make cover`)
3. `govulncheck` to find known security issues in dependencies
4. Build both Docker images and scan them with Trivy
5. `terraform fmt`, `validate` and `plan`, with the plan posted on the PR

The PR can only be merged when all of these pass (branch protection on `main`).

**CD on merge to `main`:**

1. GitHub Actions logs in to AWS with OIDC, so there are no AWS keys stored in GitHub. The role it uses can only push to ECR and update these two ECS services.
2. Build the images once, tag them with the git commit SHA, and push them to ECR. The same image goes to every environment, so what was tested is what goes live.
3. `terraform apply` for any infrastructure changes.
4. Deploy to a staging environment and run a smoke test (call `/healthz` and analyze a known page).
5. Deploy to production after a manual approval (a GitHub environment with required reviewers).
6. ECS does a rolling deploy with the deployment circuit breaker on, so if the new tasks fail their health checks it rolls back by itself.

**Dependabot** opens PRs for Go modules, the Docker base images and the GitHub Actions versions, and those go through the same CI.

**Other options:** blue/green deploys with CodeDeploy give instant rollback and let you test the new version before switching traffic, but they're more setup than a rolling deploy needs here. Argo CD or Flux would make sense only if this moved to Kubernetes.
