FROM golang:1.13-alpine AS build

RUN apk add --no-cache --virtual .deps build-base ceph-dev

WORKDIR /go/src/app
COPY . .

RUN go install --ldflags '-extldflags "-static"' ./cmd/docker-plugin-cephfs
CMD ["/go/bin/docker-plugin-cephfs"]

FROM alpine
RUN apk add --no-cache ceph-common
COPY --from=build /go/bin/docker-plugin-cephfs /usr/local/bin/
CMD ["docker-plugin-cephfs"]