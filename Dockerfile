# Build
FROM golang:1.22-alpine3.19 AS build

RUN apk --no-cache add git

WORKDIR /app
COPY . .

RUN go build -ldflags="-s -w -X 'main.Version=$(git describe --tag)'" -o pdf-server main.go


# Deploy
FROM alpine:3.19 AS deploy

LABEL maintainer="thomas@crissyfield.de"
LABEL description="PDF Server"

RUN apk --no-cache add tini \
                       tzdata

COPY --from=build /app/pdf-server /pdf-server

ENTRYPOINT [ "/sbin/tini", "--", "/pdf-server" ]
