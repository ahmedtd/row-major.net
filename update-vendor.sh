go mod tidy || exit 1
bazel run //:gazelle || exit 1
