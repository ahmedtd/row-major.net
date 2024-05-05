FROM golang:1.22 as build

WORKDIR /go/src/app
COPY . .

RUN go mod download
RUN go vet -v ./medtracker/...
RUN go test -v ./medtracker/...

RUN CGO_ENABLED=0 go build -o /go/bin/webui ./medtracker/cmd/webui

FROM gcr.io/distroless/static-debian11

# The complicated name is for compatibility with the original
# bazel-built images.
COPY --from=build /go/bin/webui /app/medtracker/cmd/webui/webui

CMD ["/app/medtracker/cmd/webui/webui"]