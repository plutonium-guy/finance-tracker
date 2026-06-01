BINARY := finance-tracker
TAILWIND := ./tailwindcss

.PHONY: run build test css clean tidy

run:
	go run ./cmd/server

build:
	CGO_ENABLED=0 go build -o $(BINARY) ./cmd/server

test:
	go test ./...

# Production CSS via the Tailwind standalone CLI (no Node). Downloads the binary
# on first use, then builds web/static/app.css from input.css (content globs and
# darkMode come from tailwind.config.js).
TAILWIND_VERSION := v3.4.17
css:
	@if [ ! -x $(TAILWIND) ]; then \
	  echo "Downloading Tailwind standalone CLI $(TAILWIND_VERSION)..."; \
	  OS=$$(uname -s); case "$$OS" in Darwin) OS=macos;; Linux) OS=linux;; esac; \
	  ARCH=$$(uname -m); case "$$ARCH" in arm64|aarch64) ARCH=arm64;; x86_64) ARCH=x64;; esac; \
	  curl -sSL -o $(TAILWIND) "https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$$OS-$$ARCH"; \
	  chmod +x $(TAILWIND); \
	fi
	$(TAILWIND) -i ./internal/web/static/input.css -o ./internal/web/static/app.css --minify

tidy:
	go mod tidy

clean:
	rm -f $(BINARY) finance.db
