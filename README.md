# Update vendor

    go mod tidy
    bazel run //:gazelle

# row-major.net

Build and push the site's container:

    bazel run //webalator:webalator_push

This will print out the tag of the pushed image.

Update manifests/row-major-web.yaml with the tag.

Run

    kubectl kustomize build manifests/ | kubectl apply -f -
