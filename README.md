# row-major.net

Build and push the site's container:

    bazel run //:site-push

This will print out the digest of the pushed image.

Update manifests/kustomization.yaml with the new digest.

Run

    kubectl kustomize build manifests/ | kubectl apply -f -
