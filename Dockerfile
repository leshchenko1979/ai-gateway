# Stage 1: build
FROM golang:alpine AS builder
WORKDIR /build
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/ ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o ai-gateway .

# Stage 2: runtime - distroless static, nonroot
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /build/ai-gateway .
# Production overrides this path via bind mount; example keeps CI/local builds working without gitignored config.yaml
COPY config.yaml.example ./config.yaml
EXPOSE 8080
ENTRYPOINT ["./ai-gateway"]
