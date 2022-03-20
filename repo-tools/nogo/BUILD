load("@io_bazel_rules_go//go:def.bzl", "nogo")

nogo(
    name = "my_nogo",
    config = ":nogo-config.json",
    vet = True,
    visibility = ["//visibility:public"],  # must have public visibility
    deps = [
        "//repo-tools/nogo/formatted:go_default_library",
    ],
)
