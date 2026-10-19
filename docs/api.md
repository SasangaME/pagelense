# API

## Backend (port 8081)

### `POST /api/v1/analyze`

```sh
curl -X POST http://localhost:8081/api/v1/analyze \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com"}'
```

Response `200`:

```json
{
  "url": "https://example.com/",
  "html_version": "HTML5",
  "title": "Example Domain",
  "headings": {"h1": 1, "h2": 0, "h3": 0, "h4": 0, "h5": 0, "h6": 0},
  "links": {"internal": 0, "external": 1, "checked": 1, "inaccessible": 0},
  "has_login_form": false
}
```

The checker tests a maximum of 100 links. Thus `checked` can be less than the total links.

### `GET /healthz`

Returns `{"status": "ok"}`.

### Errors

```json
{"error": {"status": 400, "message": "invalid URL: missing host"}}
```

| Status | Cause |
|--------|-------|
| 400    | The body or URL is not valid. |
| 403    | The URL goes to a private address. |
| 415    | The page is not HTML. |
| 502    | The api cannot get the page. |
| 504    | The page is too slow. |
| Other  | The api returns the status of the page. |

## Frontend (port 8080)

| Route           | Function |
|-----------------|----------|
| `GET /`         | Shows the form. |
| `POST /analyze` | Shows the result or the error. |
| `GET /healthz`  | Returns `{"status": "ok"}`. |
