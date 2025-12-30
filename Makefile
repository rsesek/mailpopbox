.PHONY:
all: coverage linux mac

DIRTY=$(shell test -z "`git status --untracked-files=no --porcelain`" || echo "-dirty")
GITHASH=$(shell git log --pretty='%H' -1)
LDFLAG=-o build -ldflags "-X 'src.bluestatic.org/mailpopbox/pkg/version.versionGit=$(GITHASH)$(DIRTY)'"

VERSION=$(shell sed -n -E -e 's/[[:space:]]*versionNumber = "(.*)"/\1/p' pkg/version/version.go)
PKG_BASE=mailpopbox-$(VERSION)

DOCS_FILES=README.md docs/install.md

.PHONY:
version:
	@echo $(VERSION)

clean:
	rm -rf build || true
	mkdir build

coverage:
	go test -coverprofile ./cover.out ./...
	go tool cover -html=cover.out -o cover.html

mac: clean
	GOOS=darwin GOARCH=amd64 go build $(LDFLAG) ./cmd/...
	mkdir $(PKG_BASE)
	cp build/mailpopbox $(PKG_BASE)
	cp build/mailpopbox-router $(PKG_BASE)
	cp $(DOCS_FILES) $(PKG_BASE)
	zip -r $(PKG_BASE)-mac-amd64.zip $(PKG_BASE)
	rm -rf $(PKG_BASE)

linux: clean
	GOOS=linux GOARCH=amd64 go build $(LDFLAG) ./cmd/...
	mkdir $(PKG_BASE)
	cp build/mailpopbox $(PKG_BASE)
	cp build/mailpopbox-router $(PKG_BASE)
	cp deployment/mailpopbox.service $(PKG_BASE)
	cp $(DOCS_FILES) $(PKG_BASE)
	zip -r $(PKG_BASE)-linux-amd64.zip $(PKG_BASE)
	rm -rf $(PKG_BASE)
