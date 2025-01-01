FROM localhost/coder-com/builder:latest AS build
COPY --chown=1000:1000 . /home/coder-com/irc/srvx-1.x
RUN source ${HOME}/.abuild/abuild.conf \
  && cd ${HOME}/irc/srvx-1.x \
  && abuild checksum \
  && abuild -r -s .

FROM alpine:3.21
LABEL Description="Runs a srvx 1.x daemon"
COPY --from=build /home/coder-com/.abuild/*.pub /etc/apk/keys/
COPY --from=build /home/coder-com/packages/irc /tmp/irc
RUN apk add --update -X /tmp/irc srvx \
  && rm -rf /var/cache/apk/* /tmp/irc \
  && chown -R coder-com /home/coder-com
USER coder-com
STOPSIGNAL SIGQUIT
WORKDIR /usr/share/srvx
ENTRYPOINT ["/usr/share/srvx/srvx"]
CMD ["-f"]
