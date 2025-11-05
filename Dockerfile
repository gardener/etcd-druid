# Build the manager binary
FROM golang:1.25.3 AS builder
WORKDIR /go/src/github.com/gardener/etcd-druid
COPY . .

RUN make build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian11:nonroot AS druid
WORKDIR /
COPY --from=builder /go/src/github.com/gardener/etcd-druid/bin/etcd-druid /etcd-druid
COPY charts charts
ENTRYPOINT ["/etcd-druid"]
