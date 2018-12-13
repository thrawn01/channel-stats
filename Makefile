.PHONY: release docker
.DEFAULT_GOAL := release

VERSION=$(shell cat version)

LDFLAGS="-X main.Version=$(VERSION)"

docker:
	docker build --build-arg VERSION=$(VERSION) -t thrawn01/channel-stats:$(VERSION) .
	docker tag thrawn01/channel-stats:$(VERSION) thrawn01/channel-stats:latest

release:
	GOOS=darwin GOARCH=amd64 go build -ldflags $(LDFLAGS) -o channel-stats.darwin ./cmd/channel-stats
	GOOS=linux GOARCH=amd64 go build -ldflags $(LDFLAGS) -o channel-stats.linux ./cmd/channel-stats
