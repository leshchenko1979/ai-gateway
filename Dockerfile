# Stage 1: build
FROM golang:alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o ai-gateway .

# Stage 2: runtime - distroless static, nonroot
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /build/ai-gateway .
COPY --from=builder /build/config.yaml .
EXPOSE 8080
ENTRYPOINT ["./ai-gateway"]
