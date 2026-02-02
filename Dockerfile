FROM golang:1.22 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -o /usr/bin/git-config-server .

FROM busybox:stable-glibc

COPY --from=builder /usr/bin/git-config-server /usr/bin/git-config-server

ENTRYPOINT ["git-config-server"]
