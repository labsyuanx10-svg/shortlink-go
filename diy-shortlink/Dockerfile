# Build stage
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /shortlink .

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata sqlite
RUN mkdir /data
COPY --from=builder /shortlink /shortlink
COPY templates /templates
COPY static /static
EXPOSE 8090
VOLUME /data
CMD ["/shortlink"]
