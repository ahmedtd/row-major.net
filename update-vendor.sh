go mod tidy
go mod vendor

find vendor/ -type f -name BUILD -delete
find vendor/ -type f -name BUILD.bazel -delete
find vendor/ -type f -name '*.bzl' -delete

bazel run //:gazelle
