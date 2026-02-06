.PHONY: build clean dev test

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
