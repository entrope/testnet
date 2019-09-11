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
    && adduser -D coder-com \
    && addgroup coder-com abuild \
    && echo "coder-com ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-com \
    && mkdir -p /var/cache/distfiles

USER coder-com
ENV PACKAGER="coder-com <coder-com@undernet.org>"
COPY --chown=coder-com:coder-com . /home/coder-com/irc
WORKDIR /home/coder-com/irc
RUN abuild-keygen -a -i \
    && mkdir ${HOME}/packages \
    && grep coder-com /etc/group > ${HOME}/packages/group \
    && grep coder-com /etc/passwd > ${HOME}/packages/passwd \
    && for subdir in `find * -maxdepth 0 -type d` ; do \
	cd ${subdir} \
        && abuild checksum \
        && abuild -r -s . \
        && cd - \
        ; done \
    && source ${HOME}/.abuild/abuild.conf \
    && cp ${PACKAGER_PRIVKEY}.pub ${HOME}/packages/

ENTRYPOINT ["/bin/sh"]
