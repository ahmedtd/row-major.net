set -o errexit
set -o nounset

# Prefix with STABLE_ so that these values are saved to stable-status.txt
# instead of volatile-status.txt.
# Stamped rules will be retriggered by changes to stable-status.txt, but not by
# changes to volatile-status.txt.
cat <<EOF
STABLE_GIT_COMMIT $(git rev-parse HEAD)
STABLE_IMAGE_TAG ${IMAGE_TAG:-latest}
STABLE_IMAGE_REPO ${IMAGE_REPO:-bomsync-214520}
EOF
