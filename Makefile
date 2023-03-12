.PHONY: gobuildcache

bin/bootstrap-bindown.sh: Makefile
	mkdir -p bin && \
	curl -sSfL https://github.com/WillAbides/bindown/releases/download/v3.10.1/bootstrap-bindown.sh > bin/bootstrap-bindown.sh && \
	chmod +x bin/bootstrap-bindown.sh

bin/semver-next: gobuildcache
	go build -ldflags "-s -w" -o bin/semver-next .

bin/bindown: bin/bootstrap-bindown.sh
	bin/bootstrap-bindown.sh

bin/golangci-lint: bin/bindown .bindown.yaml
	bin/bindown install $(notdir $@)

bin/goreleaser: bin/bindown .bindown.yaml
	bin/bindown install $(notdir $@)

bin/mockgen: bin/bindown .bindown.yaml
	bin/bindown install $(notdir $@)

bin/gofumpt: bin/bindown .bindown.yaml
	bin/bindown install $(notdir $@)

bin/shellcheck: bin/bindown .bindown.yaml
	bin/bindown install $(notdir $@)

GOIMPORTS_REF := v0.7.0
bin/goimports: Makefile
	GOBIN=${CURDIR}/bin \
	go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_REF)

HANDCRAFTED_REV := 082e94edadf89c33db0afb48889c8419a2cb46a9
bin/handcrafted:
	GOBIN=${CURDIR}/bin \
	go install github.com/willabides/handcrafted@$(HANDCRAFTED_REV)
