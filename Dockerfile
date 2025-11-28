# Runtime image for CIDX (using pre-built binary from CI)
FROM docker:27-cli

# Copy pre-built CIDX binary from GitHub Actions
# The binary is built in the CI pipeline and passed as a build context
COPY bin/cidx /usr/local/bin/cidx

# Set working directory
WORKDIR /workspace

# Default command
ENTRYPOINT ["cidx"]
CMD ["--help"]

# Labels for GHCR
LABEL org.opencontainers.image.source="https://github.com/cidx-org/cidx"
LABEL org.opencontainers.image.description="CIDX - CI with Declarative eXecution"
LABEL org.opencontainers.image.licenses="MIT"
