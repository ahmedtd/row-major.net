# Update vendor

    go mod tidy
    bazel run //:gazelle

# CI

I use Gitlab CI to kick off a Google Cloud Build job that builds and pushes the
containers.  I am still working on automatically updating and applying the
Kubernetes manifests.

# row-major.net

Build and push the site's container:

    bazel run //webalator:webalator_push

This will print out the tag of the pushed image.

Update manifests/row-major-web.yaml with the tag.

Run

    kubectl kustomize manifests/ | kubectl apply -f -
