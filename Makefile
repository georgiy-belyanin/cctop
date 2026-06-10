# Use a locally installed golangci-lint if it is v2, otherwise run v2 via the
# Go toolchain (slower first time, no install step).
GOLANGCI_LINT ?= $(shell golangci-lint version 2>/dev/null | grep -Eq 'version v?2\.' && echo golangci-lint || echo go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest)

.PHONY: build test vet lint fmt ci clean

build:
	go build -o cctop .

test:
	go test ./...

vet:
	go vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	$(GOLANGCI_LINT) fmt ./...

ci: vet lint test build

clean:
	rm -f cctop
