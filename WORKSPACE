load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")
load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

# Download the rules_docker repository at release v0.14.1
http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "dc97fccceacd4c6be14e800b2a00693d5e8d07f69ee187babfd04a80a9f8e250",
    strip_prefix = "rules_docker-0.14.1",
    urls = ["https://github.com/bazelbuild/rules_docker/releases/download/v0.14.1/rules_docker-v0.14.1.tar.gz"],
)

load(
    "@io_bazel_rules_docker//repositories:repositories.bzl",
    container_repositories = "repositories",
)

container_repositories()

# This is NOT needed when going through the language lang_image
# "repositories" function(s).
load("@io_bazel_rules_docker//repositories:deps.bzl", container_deps = "deps")

container_deps()

load(
    "@io_bazel_rules_docker//container:container.bzl",
    "container_pull",
)

container_pull(
    name = "nginx_base",
    registry = "index.docker.io",
    repository = "library/nginx",
    # 'tag' is also supported, but digest is encouraged for reproducibility.
    # digest = "sha256:deadbeef",
    tag = "mainline-alpine",
)

git_repository(
    name = "io_bazel_rules_python",
    commit = "fdbb17a4118a1728d19e638a5291b4c4266ea5b8",
    remote = "https://github.com/bazelbuild/rules_python.git",
)

# Enable pip support
load("@io_bazel_rules_python//python:pip.bzl", "pip_repositories")

pip_repositories()

load("@io_bazel_rules_python//python:pip.bzl", "pip_import")

# This rule translates the specified requirements.txt into
# @my_deps//:requirements.bzl, which itself exposes a pip_install method.
pip_import(
    name = "jinjaize_deps",
    requirements = "//:requirements.txt",
)

# Load the pip_install symbol for my_deps, and create the dependencies'
# repositories.
load("@jinjaize_deps//:requirements.bzl", "pip_install")

pip_install()

# Download kustomize
http_file(
    name = "kustomize",
    sha256 = "a04d79a013827c9ebb0abfe9d41cbcedf507a0310386c8d9a7efec7a36f9d7a3",
    urls = ["https://github.com/kubernetes-sigs/kustomize/releases/download/v2.0.3/kustomize_2.0.3_linux_amd64"],
)
