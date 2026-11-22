# syntax=docker/dockerfile:1
# Build: docker build -f deploy/Dockerfile --build-arg VERSION=v0.1.0 -t nithro .

FROM node:26-alpine AS web
WORKDIR /src/web
RUN npm install -g pnpm@10.33.0
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
COPY api/openapi.yaml /src/api/openapi.yaml
RUN pnpm build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/nithron/nithro/internal/buildinfo.Version=${VERSION}" \
      -o /out/nithro ./cmd/nithro

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/nithro /nithro
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/nithro"]
CMD ["all"]
