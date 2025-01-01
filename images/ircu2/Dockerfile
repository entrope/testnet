FROM localhost/coder-com/builder:latest AS build
COPY --chown=1000:1000 . /home/coder-com/irc
RUN source ${HOME}/.abuild/abuild.conf \
  && cd ~/irc/iauthd-c \
  && abuild checksum \
  && abuild -r -s . \
  && cd ../ircu2 \
  && abuild checksum \
  && abuild -r -s .

FROM alpine:3.21
LABEL Description="Runs an Undernet IRC (ircu2) daemon"
COPY --from=build /home/coder-com/.abuild/*.pub /etc/apk/keys/
COPY --from=build /home/coder-com/packages/irc /tmp/irc
RUN apk add --update -X /tmp/irc iauthd-c ircu2 \
    && rm -rf /var/cache/apk/* /tmp/irc \
    && chown -R coder-com /home/coder-com
USER coder-com
STOPSIGNAL SIGTERM
WORKDIR /home/coder-com
ENTRYPOINT ["/usr/bin/ircd"]
CMD ["-n", "-x", "5"]
