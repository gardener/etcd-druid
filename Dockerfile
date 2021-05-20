# Build the manager binary
FROM golang:1.16.4 as builder
WORKDIR /go/src/github.com/gardener/etcd-druid
COPY . .

# # cache deps before building and copying source so that we don't need to re-download as much
# # and so that source changes don't invalidate our downloaded layer
#RUN go mod download

# Build
RUN .ci/build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.12.3 AS druid
RUN apk add --update bash \
    && apk del curl
WORKDIR /
COPY --from=builder /go/src/github.com/gardener/etcd-druid/bin/linux-amd64/etcd-druid bin/.
COPY charts charts
ENTRYPOINT ["/bin/etcd-druid"]
