FROM alpine:3.11.2
LABEL Description="Alpine image for building coder-com packages"

RUN apk add --no-cache --update \
    alpine-sdk \
    autoconf \
    automake \
    byacc \
    flex \
    libevent-dev \
    libtool \
    shadow \
    && adduser -D coder-com \
    && addgroup coder-com abuild \
    && echo "coder-com ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-com \
    && mkdir -p /var/cache/distfiles

COPY --chown=coder-com:coder-com . /home/coder-com/irc
USER coder-com
RUN newgrp - abuild < ~/irc/build-all.sh

ENTRYPOINT ["/bin/sh"]
