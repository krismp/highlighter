.PHONY: build web test e2e dev-api dev-web dist smoke

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

dist: web
	SKIP_WEB=1 scripts/package.sh macos-arm64
	SKIP_WEB=1 scripts/package.sh macos-amd64
	SKIP_WEB=1 scripts/package.sh windows-amd64

smoke:
	rm -rf dist/smoke && mkdir -p dist/smoke
	hdiutil attach -quiet -nobrowse -mountpoint dist/smoke/mnt $$(ls -t dist/Highlighter-*-macos-apple-silicon.dmg | head -1)
	cp -R dist/smoke/mnt/Highlighter.app dist/smoke/ && hdiutil detach -quiet dist/smoke/mnt
	scripts/smoke-test.sh dist/smoke/Highlighter.app
