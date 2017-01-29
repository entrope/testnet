FROM alpine:3.4
LABEL Description="Alpine image for building coder-com packages"

RUN apk add --update \
    alpine-sdk \
    autoconf \
    automake \
    byacc \
    flex \
    libevent-dev \
    libtool \
    && rm -rf /var/cache/apk/* \
    && adduser -D coder-com \
    && addgroup coder-com abuild \
    && echo "coder-com ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/coder-com \
    && mkdir -p /var/cache/distfiles

USER coder-com
ENV PACKAGER="coder-com <coder-com@undernet.org>"
WORKDIR /home/coder-com/irc
COPY . ./
RUN abuild-keygen -a -i \
    && sudo chown -R coder-com:coder-com ${HOME} \
    && for subdir in `find * -maxdepth 0 -type d` ; do \
	cd ${subdir} \
        && abuild checksum \
        && abuild -r -s . \
        && cd - \
        ; done \
    && source ${HOME}/.abuild/abuild.conf \
    && cp ${PACKAGER_PRIVKEY}.pub ${HOME}/packages/

ENTRYPOINT ["/bin/sh"]
