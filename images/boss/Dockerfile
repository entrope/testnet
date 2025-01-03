FROM localhost/coder-com/builder:latest AS builder
WORKDIR /home/coder-com/src
COPY --chown=1000:1000 . /home/coder-com/src/
RUN --network=none \
  CGO_ENABLED=0 go install .

FROM alpine:3.21
LABEL Description="Testnet-controlling image for IRC test network"
COPY --from=builder /etc/passwd /etc/group /etc/
COPY --from=builder /home/coder-com/go/bin/boss /bin/boss
USER root
STOPSIGNAL SIGTERM
ENTRYPOINT ["/bin/sh", "/bin/init.sh"]
CMD []
