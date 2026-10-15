.PHONY: build test clean install-local-build uninstall-local-build path

# A tagged commit reports the tag (v0.1.0); later commits report
# <tag>-<n>-g<sha>[-dirty]; before the first tag, dev-<sha>; without git, dev.
VERSION ?= $(shell git describe --tags --dirty 2>/dev/null || { c=$$(git rev-parse --short HEAD 2>/dev/null) && echo dev-$$c; } || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS  = -X github.com/PaddleHQ/paddle-cli/internal/build.Version=$(VERSION) \
	-X github.com/PaddleHQ/paddle-cli/internal/build.Commit=$(COMMIT)

# Where `make install-local-build` creates the `paddle` symlink.
# Defaults to /usr/local/bin, which is on macOS's default PATH. Run
# `make build` first (as your normal user), then
# `make install-local-build`, or `sudo make install-local-build` if
# $(INSTALL_DIR) isn't writable (common on Apple Silicon, where /usr/local/bin
# is root-owned). The install target only symlinks; it never runs go build,
# so sudo-ing it won't leave root-owned artifacts in bin/.
# Override with:
#   make install-local-build INSTALL_DIR=~/.local/bin
INSTALL_DIR ?= /usr/local/bin

build:
	go build -ldflags '$(LDFLAGS)' \
		-o bin/paddle ./cmd/paddle

test:
	go test ./...

clean:
	rm -rf bin/

# Symlink the build into $(INSTALL_DIR) and sanity-check it's on PATH.
# Does NOT run go build; run `make build` first. This keeps the
# target safe to invoke via sudo when $(INSTALL_DIR) isn't user-writable.
install-local-build:
	@if [ ! -x "$(CURDIR)/bin/paddle" ]; then \
		echo "Build not found at bin/paddle."; \
		echo "Run 'make build' first, then re-run this target."; \
		exit 1; \
	fi
	@mkdir -p "$(INSTALL_DIR)"
	@ln -sf "$(CURDIR)/bin/paddle" "$(INSTALL_DIR)/paddle"
	@echo "Installed: $(INSTALL_DIR)/paddle -> $(CURDIR)/bin/paddle"
	@case ":$$PATH:" in \
		*":$(INSTALL_DIR):"*) \
			echo "$(INSTALL_DIR) is on your PATH."; \
			echo "Run 'paddle --version' to verify." ;; \
		*) \
			echo ""; \
			echo "WARNING: $(INSTALL_DIR) is not on your PATH."; \
			echo "  Either add it to your shell rc, or re-run with a different directory:"; \
			echo ""; \
			echo "    make install-local-build INSTALL_DIR=<dir>"; \
			echo ""; \
			echo "  Run 'make path' to see writable directories already on your PATH." ;; \
	esac

uninstall-local-build:
	@rm -f "$(INSTALL_DIR)/paddle"
	@echo "Removed $(INSTALL_DIR)/paddle"

# List directories on your PATH, marking which ones you can write to
# (i.e. which are valid candidates for INSTALL_DIR).
path:
	@echo "$$PATH" | tr ':' '\n' | awk 'NF' | while IFS= read -r dir; do \
		if [ -w "$$dir" ]; then \
			printf "  %-50s  writable\n" "$$dir"; \
		else \
			printf "  %-50s\n" "$$dir"; \
		fi; \
	done
