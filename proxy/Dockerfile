# Etapa 1: compilacion
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cacheproxy .

# Etapa 2: imagen final minima
FROM alpine:3.22
RUN adduser -D -u 10001 proxyuser
COPY --from=build /out/cacheproxy /usr/local/bin/cacheproxy
USER proxyuser
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cacheproxy"]
