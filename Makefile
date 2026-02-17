.PHONY: build install test

build:
	go build -o po ./cmd/po/

install:
	go install ./cmd/po/

test:
	go test ./...
