load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

#
# Protobuf setup
#

http_archive(
    name = "com_google_protobuf",
    sha256 = "9748c0d90e54ea09e5e75fb7fac16edce15d2028d4356f32211cfa3c0e956564",
    strip_prefix = "protobuf-3.11.4",
    urls = ["https://github.com/protocolbuffers/protobuf/archive/v3.11.4.zip"],
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()

#
# Go toolchain setup
#

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "69de5c704a05ff37862f7e0f5534d4f479418afc21806c887db544a316f3cb6b",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.27.0/rules_go-v0.27.0.tar.gz",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.27.0/rules_go-v0.27.0.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

# TODO: Remove this pinned dependency after bumping rules_go.  Rules_go bundles
# an older version of this dependency.  Sigh.
http_archive(
    name = "org_golang_google_genproto",
    patch_args = ["-p1"],
    patches = [
        # gazelle args: -repo_root . -go_prefix google.golang.org/genproto -go_naming_convention import_alias -proto disable_global
        "//repo-tools/bazel-patches:org_golang_google_genproto-gazelle.patch",
    ],
    sha256 = "c06065ed15510483aaaf3e79fa3d387f7cedd48f984586374cab44bbf4edcf88",
    strip_prefix = "go-genproto-669157292da34ccd2ff7ebc3af406854a79d61ce",
    # master, as of 2021-05-30
    urls = [
        "https://github.com/googleapis/go-genproto/archive/669157292da34ccd2ff7ebc3af406854a79d61ce.zip",
    ],
)

# TODO: Remove this pinned dependency after bumping rules_go.  Rules_go bundles
# an older version of this dependency.  Sigh.
http_archive(
    name = "go_googleapis",
    patch_args = [
        "-E",
        "-p1",
    ],
    patches = [
        # find . -name BUILD.bazel -delete
        "//repo-tools/bazel-patches:go_googleapis-deletebuild.patch",
        # set gazelle directives; change workspace name
        "//repo-tools/bazel-patches:go_googleapis-directives.patch",
        # gazelle args: -repo_root .
        "//repo-tools/bazel-patches:go_googleapis-gazelle.patch",
    ],
    sha256 = "e93e2c2217257e42b11717927d96d5799548619fbbb78cca9fa5051c39d90114",
    strip_prefix = "googleapis-1c20dcfd8052a2bea026bda36875e5b7606028db",
    # master, as of 2021-05-31
    urls = [
        "https://github.com/googleapis/googleapis/archive/1c20dcfd8052a2bea026bda36875e5b7606028db.zip",
    ],
)

go_rules_dependencies()

go_register_toolchains(version = "1.15")

http_archive(
    name = "bazel_gazelle",
    sha256 = "62ca106be173579c0a167deb23358fdfe71ffa1e4cfdddf5582af26520f1c66f",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.23.0/bazel-gazelle-v0.23.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.23.0/bazel-gazelle-v0.23.0.tar.gz",
    ],
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("//:go_dependencies.bzl", "go_dependencies")

# gazelle:repository_macro go_dependencies.bzl%go_dependencies
go_dependencies()

gazelle_dependencies()

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "85ffff62a4c22a74dbd98d05da6cf40f497344b3dbf1e1ab0a37ab2a1a6ca014",
    strip_prefix = "rules_docker-0.23.0",
    urls = ["https://github.com/bazelbuild/rules_docker/releases/download/v0.23.0/rules_docker-v0.23.0.tar.gz"],
)

load(
    "@io_bazel_rules_docker//repositories:repositories.bzl",
    container_repositories = "repositories",
)

container_repositories()

load("@io_bazel_rules_docker//repositories:deps.bzl", container_deps = "deps")

container_deps()

load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repos = "repositories",
)

go_image_repos()

# Setup nodejs (for javascript tests)

# http_archive(
#     name = "build_bazel_rules_nodejs",
#     sha256 = "f690430f4d4cc403b5c90d0f0b21842183b56b732fff96cfe6555fe73189906a",
#     urls = ["https://github.com/bazelbuild/rules_nodejs/releases/download/5.0.1/rules_nodejs-5.0.1.tar.gz"],
# )

# load("@build_bazel_rules_nodejs//:index.bzl", "node_repositories")

# node_repositories()
