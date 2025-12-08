GOFILES := $(shell find . -name '*.go')
PKGS := ./...

.PHONY: fmt fmt-check lint staticcheck test check

fmt:
	gofmt -w $(GOFILES)

fmt-check:
	@fmt_files=$$(gofmt -l $(GOFILES)); \
	if [ -n "$$fmt_files" ]; then \
		echo "Go files not formatted:"; \
		echo "$$fmt_files"; \
		exit 1; \
	fi

lint:
	go vet $(PKGS)

staticcheck:
	staticcheck $(PKGS)

test:
	go test $(PKGS)

check: fmt-check lint staticcheck test
