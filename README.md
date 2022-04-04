# Update vendor

    go mod tidy
    bazel run //:gazelle
    bazel run //:gazelle -- update-repos -from_file=go.mod

