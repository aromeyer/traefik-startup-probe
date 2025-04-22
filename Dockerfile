FROM golang:1.20 AS build
WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go get -d -v ./...
COPY . .
RUN go build -o /go/bin/app

FROM debian:bookworm-slim
RUN apt-get update && apt-get install --yes ca-certificates
RUN groupadd -r app && useradd --no-log-init -r -g app app
USER app
COPY --from=build /go/bin/app /
ENV APP_ADDR=":8083"
EXPOSE 8083
ENTRYPOINT ["/app"]
