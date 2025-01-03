#! /bin/sh -e

DOCKER=${DOCKER-podman}
GO=${GO-go}
MAKE=${MAKE-make}

${MAKE}
${DOCKER} build -t localhost/coder-com/builder images/builder
${MAKE} build
${GO} run tools/checkdeps.go
