GIT = git
GO = go
DOCKER = podman
IAUTH_VERSION = 1.0.5
SRVX_VERSION = 1.4.0-rc3

.PHONY: clean

all: images orchestrate/orchestrate

orchestrate/orchestrate: orchestrate/main.go
	cd orchestrate && $(GO) build

images: packages
	for pkg in boss entity ircd srvx ; do \
		if test -z `$(DOCKER) images -q testnet:$$pkg` ; then \
			$(DOCKER) build packages -f Dockerfile.$$pkg -t testnet:$${pkg} ; \
		fi; \
	done

packages: builder/Dockerfile \
	builder/go-testnet/go.mod \
	builder/iauth/iauthd-c-$(IAUTH_VERSION).tar.gz \
	builder/ircu2/ircu2.tar.gz \
	builder/srvx1/srvx-$(SRVX_VERSION).tar.gz
	rm -fr packages
	$(DOCKER) build -t coder-com/builder builder
	CID=`$(DOCKER) create coder-com/builder` && \
	$(DOCKER) cp $$CID:/home/coder-com/packages . && \
	$(DOCKER) rm $$CID > /dev/null

builder/iauth/iauthd-c-$(IAUTH_VERSION).tar.gz: +iauthd-c/Makefile
	$(MAKE) -C +iauthd-c dist
	rm -f $@ && ln +iauthd-c/iauthd-c-$(IAUTH_VERSION).tar.gz $@

builder/ircu2/ircu2.tar.gz: ircu2/configure
	tar czf $@ ircu2

builder/srvx1/srvx-$(SRVX_VERSION).tar.gz: +srvx-1.x/Makefile
	$(MAKE) -C +srvx-1.x dist
	rm -f $@ && ln +srvx-1.x/srvx-$(SRVX_VERSION).tar.gz $@

+iauthd-c/Makefile: iauthd-c/configure
	test -d +iauthd-c || mkdir +iauthd-c
	cd +iauthd-c && ../iauthd-c/configure

+srvx-1.x/Makefile: srvx-1.x/configure
	test -d +srvx-1.x || mkdir +srvx-1.x
	cd +srvx-1.x && ../srvx-1.x/configure --enable-maintainer-mode

iauthd-c/configure: iauthd-c/configure.ac
	autoreconf -Wall -i iauthd-c

srvx-1.x/configure: srvx-1.x/configure.ac
	autoreconf -Wall -i srvx-1.x

iauthd-c/configure.ac ircu2/configure srvx-1.x/configure.ac \
	builder/go-testnet/go.mod:
	$(GIT) submodule update --init

clean:
	rm -fr packages \
	orchestrate/orchestrate \
	builder/iauth/iauthd-c-$(IAUTH_VERSION).tar.gz \
	builder/ircu2/ircu2.tar.gz \
	builder/srvx1/srvx-$(SRVX_VERSION).tar.gz
