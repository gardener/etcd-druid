# Build the manager binary
FROM golang:1.19.2 as builder
WORKDIR /go/src/github.com/gardener/etcd-druid
COPY . .

# # cache deps before building and copying source so that we don't need to re-download as much
# # and so that source changes don't invalidate our downloaded layer
#RUN go mod download

# Build
RUN .ci/build

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian11:nonroot AS druid
WORKDIR /
COPY --from=builder /go/src/github.com/gardener/etcd-druid/bin/etcd-druid /etcd-druid
COPY charts charts
ENTRYPOINT ["/etcd-druid"]
