FROM golang:1.22

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -o /usr/bin/git-config-server .

ENTRYPOINT ["git-config-server"]
