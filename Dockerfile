# Runtime image for CIDX (using pre-built binary from CI)
#
# Docker Hub, not dhi.io. The catalogue pins `dhi.io/docker:29-cli` for the
# docker-buildx preset, but this image is published for third parties and
# dhi.io answers 401 without a subscription entitlement -- the same reasoning
# that kept the GitLab generator off it (#294). `docker:27-cli` was built
# 2025-02-12 and carries Docker 27.5.1, a series that is EOL upstream; 29-cli
# is the current one. Pinned by digest, rule 1 of the supply-chain policy.
FROM docker:29-cli@sha256:27a51d5ab1cd38d9eeaba7b415b8c07bc10c31e1cf1ec8d78f6413fcfab3f44f

# Copy pre-built CIDX binary from GitHub Actions
# The binary is built in the CI pipeline and passed as a build context.
# `.dockerignore` re-includes this exact path out of an otherwise ignored
# bin/ -- see the comment there before touching either (#281). The binary must
# also be statically linked: this base is Alpine/musl, and release.yml builds
# with CGO_ENABLED=0 for that reason.
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
