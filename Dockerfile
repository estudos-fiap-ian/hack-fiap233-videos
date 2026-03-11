FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -o /videos .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /videos /videos
EXPOSE 8080
CMD ["/videos"]
