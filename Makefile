GIT = git
GO = go
DOCKER = podman
IAUTH_VERSION = 1.0.5
SRVX_VERSION = 1.4.0-rc3

.PHONY: clean

all: packages images orchestrate/orchestrate

orchestrate/orchestrate: orchestrate/main.go
	cd orchestrate && $(GO) build

images: packages
	for pkg in boss ircu2 srvx-1.x ; do \
		if test -z `$(DOCKER) images -q localhost/testnet/$$pkg` ; then \
			$(DOCKER) build packages -f Dockerfile.$$pkg -t localhost/testnet/$${pkg} ; \
		fi; \
	done

packages: Dockerfile.buildimg \
	Dockerfile.builder \
	builder/go-testnet/go.mod \
	builder/iauthd-c/iauthd-c-$(IAUTH_VERSION).tar.gz \
	builder/ircu2/ircu2.tar.gz \
	builder/srvx-1.x/srvx-$(SRVX_VERSION).tar.gz
	rm -fr packages
	if ! CID=`$(DOCKER) create localhost/coder-com/builder` ; then \
		$(DOCKER) build builder -f Dockerfile.builder -t localhost/coder-com/builder && \
		CID=`$(DOCKER) create localhost/coder-com/builder` ; \
	fi && \
	$(DOCKER) cp $$CID:/home/coder-com/packages . && \
	$(DOCKER) rm $$CID > /dev/null

images/ircu2/iauthd-c/iauthd-c-$(IAUTH_VERSION).tar.gz: +iauthd-c/Makefile
	$(MAKE) -C +iauthd-c dist
	rm -f $@ && ln +iauthd-c/iauthd-c-$(IAUTH_VERSION).tar.gz $@

images/ircu2/ircu2/ircu2.tar.gz: ircu2/configure
	tar czf $@ ircu2

images/srvx-1.x/srvx-$(SRVX_VERSION).tar.gz: +srvx-1.x/Makefile
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

clean-tests:
	for dir in tests/*/* ; do if test -d $$dir ; then rm -r $$dir ; fi ; done
	rm -fr coverage/*/gcda coverage/*/gcno coverage/*/lcov.dat \
	tests/*/compose.yaml tests/*/irc.script

clean: clean-tests
	rm -fr packages \
	orchestrate/orchestrate \
	builder/iauthd-c/iauthd-c-$(IAUTH_VERSION).tar.gz \
	builder/ircu2/ircu2.tar.gz \
	builder/srvx-1.x/srvx-$(SRVX_VERSION).tar.gz
