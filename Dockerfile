## Build stage
FROM golang:1.25-bookworm AS build

RUN apt-get update && apt-get install -y zip

WORKDIR /go/src/server

# Копируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем код
COPY . .

# tidy принудительно вычистит лишние импорты перед сборкой
RUN go mod tidy
RUN go build -v -o /go/bin/app main.go

# Подготовка конфигов
RUN mkdir -p /go/bin/config && \
    cp -r ./appdata/* /go/bin/config/ || true

## Deploy stage
FROM debian:bookworm-slim 
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=build /go/bin /go/bin
WORKDIR /go/bin
EXPOSE 8080
ENTRYPOINT ["/go/bin/app"]
CMD ["-v", "web"]

