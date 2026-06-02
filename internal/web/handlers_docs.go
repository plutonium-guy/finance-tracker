package web

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.json
var openAPISpec []byte

// OpenAPISpec serves the OpenAPI 3 document.
func (h *Handler) OpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(openAPISpec)
}

// APIDocs serves a self-contained Swagger UI (assets are vendored under /static).
func (h *Handler) APIDocs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerHTML))
}

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Finance Tracker API — Swagger UI</title>
  <link rel="stylesheet" href="/static/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="/static/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/api/openapi.json',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis],
    });
  </script>
</body>
</html>`
