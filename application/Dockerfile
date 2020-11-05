# Dockerfile
FROM golang:1.10-alpine3.7

WORKDIR /go/src/app

COPY . .

RUN apk add --no-cache curl git

RUN go get github.com/lib/pq

RUN go get github.com/prometheus/client_golang/prometheus/promhttp

RUN go build -o go-pg-crud

EXPOSE 8080/tcp

ENTRYPOINT ["/go/src/app/go-pg-crud"]

