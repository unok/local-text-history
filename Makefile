.PHONY: build clean dev test \
	build-release-linux-amd64 build-release-linux-arm64 \
	build-release-darwin-amd64 build-release-darwin-arm64

LDFLAGS := -s -w

build: web-build go-build

web-build:
	cd web && npm ci && npm run build

go-build:
	CGO_ENABLED=1 go build -o bin/file-history ./cmd/file-history

dev:
	cd web && npm run dev &
	go run ./cmd/file-history --config config.example.json

clean:
	rm -rf bin/ web/dist/

test:
	CGO_ENABLED=1 go test ./...

build-release-linux-amd64:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=gcc \
		go build -ldflags '$(LDFLAGS) -extldflags "-static"' \
		-o bin/file-history-linux-amd64 ./cmd/file-history

build-release-linux-arm64:
	CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc \
		go build -ldflags '$(LDFLAGS) -extldflags "-static"' \
		-o bin/file-history-linux-arm64 ./cmd/file-history

build-release-darwin-amd64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
		go build -ldflags '$(LDFLAGS)' \
		-o bin/file-history-darwin-amd64 ./cmd/file-history

build-release-darwin-arm64:
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
		go build -ldflags '$(LDFLAGS)' \
		-o bin/file-history-darwin-arm64 ./cmd/file-history
