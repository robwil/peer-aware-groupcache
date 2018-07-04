# build stage
FROM golang:1.10-alpine AS build-env
#RUN apk add --no-cache git
RUN mkdir -p /go/src/github.com/robwil/peer-aware-groupcache
WORKDIR /go/src/github.com/robwil/peer-aware-groupcache
COPY . .
RUN go build -o peer-aware-groupcache

# final stage
FROM alpine:3.7
WORKDIR /app
RUN apk add --no-cache ca-certificates apache2-utils
COPY --from=build-env /go/src/github.com/robwil/peer-aware-groupcache/peer-aware-groupcache /app/
EXPOSE 5000
ENTRYPOINT ./peer-aware-groupcache