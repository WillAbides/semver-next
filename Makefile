GOCMD=go
GOBUILD=$(GOCMD) build

.PHONY: gobuildcache

bin/semver-next: gobuildcache
	go build -o bin/semver-next .
bins += bin/semver-next

bin/bindownloader:
	./script/bootstrap-bindownloader -b bin
bins += bin/bindownloader

bin/golangci-lint: bin/bindownloader
	bin/bindownloader $@
bins += bin/golangci-lint

bin/gobin: bin/bindownloader
	bin/bindownloader $@
bins += bin/gobin

bin/goreleaser: bin/bindownloader
	bin/bindownloader $@
bins += bin/goreleaser

MOCKGEN_REF := 9fa652df1129bef0e734c9cf9bf6dbae9ef3b9fa
bin/mockgen: bin/gobin
	GOBIN=${CURDIR}/bin \
	bin/gobin github.com/golang/mock/mockgen@$(MOCKGEN_REF);
bins += bin/mockgen

GOIMPORTS_REF := 8aaa1484dc108aa23dcf2d4a09371c0c9e280f6b
bin/goimports: bin/gobin
	GOBIN=${CURDIR}/bin \
	bin/gobin golang.org/x/tools/cmd/goimports@$(GOIMPORTS_REF)
bins += bin/goimports

.PHONY: clean
clean:
	rm -rf $(bins) $(cleanup_extras)
