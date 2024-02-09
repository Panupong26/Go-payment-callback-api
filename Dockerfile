# Dockerfile
FROM golang:1.21.6-alpine3.18

WORKDIR /app

COPY . .

RUN go build -o main .

EXPOSE 8000

CMD ["./main"]