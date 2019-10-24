FROM golang:1.13-alpine AS build

RUN apk add --no-cache --virtual .deps build-base ceph-dev

WORKDIR /go/src/app
COPY . .

RUN go install --ldflags '-extldflags "-static"'
CMD ["/go/bin/docker-plugin-ceph"]

FROM alpine
RUN apk add --no-cache ceph-common
COPY --from=build /go/bin/docker-plugin-ceph /usr/local/bin/
CMD ["docker-plugin-ceph"]