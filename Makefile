.PHONY: build test deploy

build:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/cobalt-telegram-bot ./cmd/cobalt-telegram-bot

test:
	go test ./...

deploy:
	./scripts/deploy.sh
