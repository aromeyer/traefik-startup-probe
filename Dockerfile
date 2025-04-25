FROM golang:1.20 AS build
WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go get -d -v ./...
COPY . .
RUN go build -o /go/bin/app

FROM debian:bookworm-slim
ARG HTTP_SERVER_PORT=8083
ENV HTTP_SERVER_ADDR=":${HTTP_SERVER_PORT}"

RUN apt-get update && apt-get install --yes ca-certificates
RUN groupadd -r app && useradd --no-log-init -r -g app app

USER app
COPY --from=build /go/bin/app /

EXPOSE ${HTTP_SERVER_PORT}
ENTRYPOINT ["/app"]
