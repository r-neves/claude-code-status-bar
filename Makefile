BINARY  := claude-code-status-bar
PREFIX  ?= $(HOME)/.claude
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := build

.PHONY: build
build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .

.PHONY: test
test:
	go test ./...

.PHONY: check
check: fmt vet test

.PHONY: fmt
fmt:
	gofmt -l -w .

.PHONY: vet
vet:
	go vet ./...

# Build and copy the binary. Pass CONFIGURE=1 to also point settings.json at it.
.PHONY: install
install: build
	@mkdir -p $(PREFIX)
	install -m 0755 $(BINARY) $(PREFIX)/$(BINARY)
	@echo "installed $(PREFIX)/$(BINARY)"
ifeq ($(CONFIGURE),1)
	@./install.sh --configure --skip-build
else
	@echo
	@echo "To wire it up, add this to $(PREFIX)/settings.json:"
	@echo '  "statusLine": { "type": "command", "command": "$(PREFIX)/$(BINARY)", "padding": 0 }'
	@echo
	@echo "Or run: make install CONFIGURE=1"
endif

.PHONY: uninstall
uninstall:
	rm -f $(PREFIX)/$(BINARY)
	@echo "removed $(PREFIX)/$(BINARY); the statusLine entry in settings.json is left untouched"

# Render the three layouts with sample data.
.PHONY: demo
demo: build
	@for l in compact ultra full; do \
		printf '\n\033[1m-- %s --\033[0m\n' $$l; \
		./$(BINARY) --layout $$l; \
		printf '\n'; \
	done

.PHONY: clean
clean:
	rm -f $(BINARY)
