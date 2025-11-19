# CIDX Bootstrap Makefile
#
# This minimal Makefile is ONLY for the initial bootstrap build of CIDX.
# After building CIDX once, use CIDX itself for all other operations:
#   - bin/cidx run test      (instead of make test)
#   - bin/cidx run build     (instead of make build)
#   - bin/cidx run ci        (instead of make all)
#   - bin/cidx run pre-push  (instead of make dev)

.PHONY: build clean help

VERSION=$(shell cat VERSION)
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Bootstrap build - compile CIDX for the first time
build:
	@echo "🔨 Bootstrapping CIDX v$(VERSION)..."
	@mkdir -p bin
	@go build $(LDFLAGS) -o bin/cidx ./cmd/cidx
	@echo "✅ Bootstrap complete: bin/cidx"
	@echo ""
	@echo "Now use CIDX to build CIDX:"
	@echo "  bin/cidx run test       # Run tests"
	@echo "  bin/cidx run build      # Build with version injection"
	@echo "  bin/cidx run ci         # Full CI pipeline"
	@echo "  bin/cidx run pre-push   # Quick validation"

# Clean build artifacts
clean:
	@echo "🧹 Cleaning..."
	@rm -rf bin/
	@go clean

# Help
help:
	@echo "CIDX Bootstrap Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build   - Bootstrap build of CIDX (first time only)"
	@echo "  clean   - Remove build artifacts"
	@echo "  help    - Show this help"
	@echo ""
	@echo "After bootstrap, use CIDX itself:"
	@echo "  bin/cidx run test       # Tests"
	@echo "  bin/cidx run build      # Build with version"
	@echo "  bin/cidx run ci         # Full CI"
	@echo "  bin/cidx run pre-push   # Quick check"
