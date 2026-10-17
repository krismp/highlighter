.PHONY: build web test e2e dev-api dev-web

build: web
	go build -o bin/highlighter ./cmd/highlighter

web:
	cd web && pnpm install --frozen-lockfile && pnpm build
	touch internal/ui/dist/.gitkeep

test:
	go test -race ./...

e2e:
	go test -tags e2e -count=1 -timeout 5m ./e2e/...

dev-api:
	go run ./cmd/highlighter -no-open

dev-web:
	cd web && pnpm dev
