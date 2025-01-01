FROM localhost/coder-com/builder:latest AS builder
WORKDIR /home/coder-com/src
COPY --chown=1000:1000 . /home/coder-com/src/
RUN --network=none \
  CGO_ENABLED=0 go install .

FROM scratch
LABEL Description="Testnet-controlling image for IRC test network"
COPY --from=builder /etc/passwd /etc/group /etc/
COPY --from=builder /home/coder-com/go/bin/boss /bin/boss
USER coder-com
STOPSIGNAL SIGTERM
ENTRYPOINT ["/bin/boss"]
CMD []
