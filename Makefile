.PHONY:
all: coverage mac linux

DIRTY=$(shell [[ -z `git status --untracked-files=no --porcelain` ]] || echo "-dirty")
GITHASH=$(shell git log --pretty='%H' -1)

LDFLAG=-ldflags "-X 'main.versionGit=$(GITHASH)$(DIRTY)'"

coverage:
	go test -coverprofile ./cover.out ./...
	go tool cover -html=cover.out -o cover.html

mac:
	GOOS=darwin GOARCH=amd64 go build -o mailpopbox-mac-amd64 $(LDFLAG)

linux:
	GOOS=linux GOARCH=amd64 go build -o mailpopbox-linux-amd64 $(LDFLAG)
