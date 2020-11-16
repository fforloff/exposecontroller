FROM golang as build-stage
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY controller controller
COPY exposestrategy exposestrategy
RUN go build -o exposecontroller

FROM golang as production-stage
LABEL MAINTAINER="Aurelien Lambert <aure@olli-ai.com>"

COPY --from=build-stage /app/exposecontroller /exposecontroller
ENTRYPOINT  ["/exposecontroller", "--daemon"]
