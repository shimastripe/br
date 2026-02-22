GO ?= go
GOCACHE ?= /tmp/go-build
GOMODCACHE ?= /tmp/gomodcache
GOENV = GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE)

default: test

update-spec:
	./tools/update_spec.sh

generate:
	$(GOENV) $(GO) run ./internal/gen/openapi_gen.go

test:
	$(GOENV) $(GO) test ./...

check-generated:
	$(MAKE) generate
	git diff --exit-code -- internal/cli/generated/commands_gen.go
