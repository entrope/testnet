FROM alpine:3
LABEL Description="Alpine image for building coder-com packages"

RUN apk add --update \
    alpine-sdk \
    autoconf \
    automake \
    byacc \
    clang17 \
    compiler-rt \
    flex \
    go \
    libevent-dev \
    libtool \
    openssl-dev \
    shadow \
    sudo \
    && adduser -D coder-com \
    && addgroup coder-com abuild \
    && echo "coder-com ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-com \
    && mkdir -p /var/cache/distfiles

USER coder-com:abuild
COPY --chown=coder-com:coder-com . /home/coder-com/irc
RUN --mount=type=cache,target=/var/cache/apk \
  --mount=type=cache,target=/root/.cache/go-build,mode=0777 \
  cd ~/irc ;\
  PACKAGER="coder-com <coder-com@undernet.org>" ;\
  CGO_ENABLED=0 ;\
  export PACKAGER CGO_ENABLED ;\
  abuild-keygen -ain ;\
  mkdir ${HOME}/packages ;\
  grep coder-com /etc/group > ${HOME}/packages/group ;\
  grep coder-com /etc/passwd > ${HOME}/packages/passwd ;\
  cd go-testnet ;\
  go install -v ./cmd/entity ./cmd/boss ;\
  cd - ;\
  for subdir in `find * -maxdepth 0 -type d` ; do \
    if test ${subdir} != go-testnet ; then \
      cd ${subdir} ;\
      abuild checksum ;\
      abuild -r -s . ;\
      cd - ;\
    fi ;\
  done ;\
  source ${HOME}/.abuild/abuild.conf ;\
  cp ${PACKAGER_PRIVKEY}.pub ${HOME}/packages/

ENTRYPOINT ["/bin/sh"]
