# bash container-push.bash path/to/Dockerfile registry imagename path/to/kustomization
#
# This script handles building the container image, pushing the container to the
# registry, and editing our kubernetes manifest kustomization to use the image's
# digest.

dockerfile="${1}"
image_name="${2}"
kustomization_dir="${3}"

# Generate a random tag for the image, so we can uniquely track it throughout
# this process.
tag=$(head -c 16 /dev/urandom | xxd -p)

tagged_image="${image_name}:${tag}"

# Run the build, labeling the resulting image with our tag.
docker build --file="${dockerfile}" --tag="${tagged_image}" . || exit 1

# Push the image to the registry.  The registry will compute a cryptographic
# digest for the image.  The digest value is different for different registries.
docker push "${tagged_image}" || exit 1

# Pushing the image filled in the digest value on our local image tag.  Retrieve
# it.
digest=$(docker image inspect -f '{{index .RepoDigests 0}}' "${tagged_image}" || exit 1)
echo Extracted digest: "${digest}"

# Write the digest into our kustomizations file, so that our kubernetes
# deployments will pull the specific image we built.
#
# For whatever reason, `kustomize edit` only operates on the current directory
pushd "${kustomization_dir}"
kustomize edit set imagetag "${digest}" || exit 1
popd
