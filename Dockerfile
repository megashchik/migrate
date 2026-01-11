FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o migrate .

FROM alpine:3.22

RUN apk --no-cache add ca-certificates

COPY --from=builder /app/migrate .

RUN chmod +x migrate

ENTRYPOINT ["./migrate"]