FROM golang as build-stage
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY controller controller
COPY exposestrategy exposestrategy
RUN go build -o exposecontroller

FROM alpine as production-stage
LABEL MAINTAINER="Aurelien Lambert <aure@olli-ai.com>"

COPY --from=build-stage /app/exposecontroller /exposecontroller
RUN apk --no-cache upgrade && \
    mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2 

ENTRYPOINT  ["/exposecontroller", "--daemon"]
