FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=readonly go build -trimpath -o /app ./cmd/app

# One image runs three commands: `serve` (the default), `worker` and `migrate`.
FROM gcr.io/distroless/static-debian12:nonroot

# Reported by /api/version. CI passes the release version (or sha-<short>),
# the commit and the branch; "unknown" for anyone who builds without them.
ARG APP_VERSION=unknown
ARG GIT_COMMIT=unknown
ARG GIT_BRANCH=unknown
ENV APP_VERSION=$APP_VERSION
ENV GIT_COMMIT=$GIT_COMMIT
ENV GIT_BRANCH=$GIT_BRANCH

COPY --from=builder /app /app

EXPOSE 8080 8081
ENTRYPOINT ["/app"]
CMD ["serve"]
