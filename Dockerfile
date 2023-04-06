# Build the manager binary
FROM --platform=$BUILDPLATFORM golang:1.20.3 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

ARG TARGETOS TARGETARCH

# Build
RUN --mount=type=cache,target=/root/.cache/go-build \
        --mount=type=cache,target=/go/pkg \
        GOOS=$TARGETOS GOARCH=$TARGETARCH go build -a -o msm-controller cmd/msm-cp/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
#FROM gcr.io/distroless/static:nonroot
FROM ubuntu
WORKDIR /
COPY --from=builder /workspace/msm-controller .
#USER nonroot:nonroot

ENTRYPOINT ["/msm-controller", "-grpcPort", "9000", "-loglevel", "5"]
