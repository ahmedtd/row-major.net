# This Dockerfile assumes that the binary has already been built using the go
# toolchain and placed at medtracker/cmd/poller/poller

FROM gcr.io/distroless/static-debian11

# The complicated name is for compatibility with the original
# bazel-built images.
COPY webui /app/medtracker/cmd/poller/poller

CMD ["/app/medtracker/cmd/poller/poller"]