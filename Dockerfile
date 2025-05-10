FROM golang:1.23-alpine

WORKDIR /app

COPY . .

RUN go mod download

RUN go build -o publisher_subscriber ./cmd/publisher_subscriber/grpc/main.go


CMD ["./publisher_subscriber"]