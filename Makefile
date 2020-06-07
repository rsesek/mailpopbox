.PHONY:
all: coverage mac linux

coverage:
	go test -coverprofile ./cover.out ./...
	go tool cover -html=cover.out -o cover.html

mac:
	GOOS=darwin GOARCH=amd64 go build -o mailpopbox-mac-amd64

linux:
	GOOS=linux GOARCH=amd64 go build -o mailpopbox-linux-amd64
