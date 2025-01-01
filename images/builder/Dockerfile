FROM alpine:3.21
LABEL Description="Alpine image for building coder-com packages"

RUN --mount=type=cache,target=/var/cache/apk \
  apk add --update \
    alpine-sdk \
    autoconf \
    automake \
    byacc \
    clang19 \
    compiler-rt \
    flex \
    go \
    libevent-dev \
    libgcc \
    libtool \
    openssl-dev \
    shadow \
    sudo \
  && adduser -D coder-com \
  && addgroup coder-com abuild \
  && echo "coder-com ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-com \
  && mkdir -p /var/cache/distfiles

USER coder-com:abuild
ENV PACKAGER="coder-com <coder-com@undernet.org>"
RUN abuild-keygen -ain \
  && mkdir -p ${HOME}/packages/irc

ENTRYPOINT ["/bin/sh"]
