DOCKER = podman
GIT = git
GO = go

.PHONY: all build clean clean-all coverage

TARBALLS = \
	images/ircu2/iauthd-c/iauthd-c.tar.gz \
	images/ircu2/ircu2/ircu2.tar.gz \
	images/srvx-1.x/srvx-1.x.tar.gz

COVERAGE = \
	coverage/iauthd-c/html/index.html \
	coverage/ircu2/html/index.html \
	coverage/srvx-1.x/html/index.html

all: orchestrate/orchestrate $(TARBALLS) .deps
coverage: $(COVERAGE)

# generic targets

iauthd-c/configure.ac ircu2/configure srvx-1.x/configure.ac: .gitmodules
	$(GIT) submodule update --init

clean:
	rm -f orchestrate/orchestrate coverage/*/lcov.dat coverage/*/*-gcno.tar.bz2 tests/*/compose.yaml tests/*/irc.script
	rm -fr coverage/*/gcda coverage/*/gcno coverage/*/html
	for dir in tests/*/* ; do if test -d $$dir ; then rm -r $$dir ; fi ; done

clean-all: clean
	rm -f $(TARBALLS)

build: $(TARBALLS)
	$(DOCKER) build --target build -t localhost/coder-com/ircu2:build images/ircu2
	$(DOCKER) build -t localhost/coder-com/ircu2:latest images/ircu2
	$(DOCKER) build --target build -t localhost/coder-com/srvx-1.x:build images/srvx-1.x
	$(DOCKER) build -t localhost/coder-com/srvx-1.x:latest images/srvx-1.x

# orchestrate

orchestrate/orchestrate: orchestrate/orchestrate.go
	$(GO) build -C orchestrate

# iauthd-c

iauthd-c/configure: iauthd-c/configure.ac
	autoreconf -Wall -i iauthd-c

+iauthd-c/Makefile: iauthd-c/configure
	test -d +iauthd-c || mkdir +iauthd-c
	cd +iauthd-c && ../iauthd-c/configure

images/ircu2/iauthd-c/iauthd-c.tar.gz: +iauthd-c/Makefile
	$(MAKE) -C +iauthd-c dist distdir=iauthd-c
	rm -f $@ && ln +iauthd-c/iauthd-c.tar.gz $@

coverage/iauthd-c/html/index.html: coverage/iauthd-c/lcov.dat
	cd coverage/iauthd-c && ./coverage.sh html

# ircu2

images/ircu2/ircu2/ircu2.tar.gz: ircu2/configure
	tar czf $@ ircu2

coverage/ircu2/html/index.html: coverage/ircu2/lcov.dat
	cd coverage/ircu2 && ./coverage.sh html

# srvx-1.x

srvx-1.x/configure: srvx-1.x/configure.ac
	autoreconf -Wall -i srvx-1.x

+srvx-1.x/Makefile: srvx-1.x/configure
	test -d +srvx-1.x || mkdir +srvx-1.x
	cd +srvx-1.x && ../srvx-1.x/configure --enable-maintainer-mode

images/srvx-1.x/srvx-1.x.tar.gz: +srvx-1.x/Makefile
	$(MAKE) -C +srvx-1.x dist distdir=srvx-1.x
	rm -f $@ && ln +srvx-1.x/srvx-1.x.tar.gz $@

coverage/srvx-1.x/html/index.html: coverage/srvx-1.x/lcov.dat
	cd coverage/srvx-1.x && ./coverage.sh html

# tools/checkdeps.go will populate .deps.
include .deps
