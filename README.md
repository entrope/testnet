# What Is It?

This repository contains scripts to build a containerized IRC test
network based on a scriptable network driver and
ircu2, iauthd-c, and srvx 1.x.

## A Really Quick Start

Run `make`.
This will generate lots of output, culminating in the creation of
several container images.
Then you can run `./orchestrate/orchestrate orchestrate/simple-irc.tmpl`
to set up and run a simple test network.

## The Longer Story

The `Makefile` in this directory will build a development container
image and copy Alpine Linux packages out of the image.
It uses git submodules to store copies of the source code to use;
you can delete the tarballs in `builder/*/*.tar.gz` to generate new ones
from the working trees.
The generated packages contain debug symbols for easier debugging, and
have code coverage enabled.

The default target for the Makefile (`images`) builds the `packages`
directory and then runs `podman build -f Dockerfile.${image}` for any
testnet image that does not exist in the local repository.

## Debugging Crashes

If you need to debug a crash inside the testnet, you will probably want
to do something like `apk add -U gdb` (as root) to install gdb in the
container(s) seeing crashes.

Linux users may also need to adjust `/proc/sys/kernel/core_pattern` to
ensure corefiles are dumped in the container.
(If `core_pattern` says to invoke an executable, that executable will
probably not exist in the container; the easiest way is to reset it to
`core` or some formatted pattern, but be aware that this sysctl is not
virtualized as of Linux 4.7.)

## Host-Side Development

The `.gitignore` file ignores `/+*/` to support the creation of build
directories on the host system -- for example, `+iauthd-c` for the
iauthd-c submodule.

## TODO

- Finish writing the orchestrator.
- Allow running the IRC software under valgrind or with sanitizer(s).
  - Ideally allow selection of instrumentation: branch and coverage
    profiling, sanitizers, GCC's -fanalyzer, clang's --analyze,
    valgrind, maybe others.
