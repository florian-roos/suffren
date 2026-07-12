FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o suffren ./cmd/suffren


FROM alpine:latest  

RUN apk --no-cache add ca-certificates

WORKDIR /app
RUN mkdir -p /app/data

COPY --from=builder /app/suffren .

EXPOSE 8080

CMD ["./suffren", "--api"]
