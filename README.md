# What Is It?

This repository contains scripts to build a containerized IRC test
network based on a scriptable network driver and
ircu2, iauthd-c, and srvx 1.x.

## A Really Quick Start

Run `sh bootstrap.sh`.
Then you can run `./orchestrate/orchestrate orchestrate/simple-irc.tmpl`
to set up and run a simple test network.

## The Longer Story

`bootstrap.sh` does several things:

1. Use `tools/genninja.go` to create a `ninja.build` file.
1. Runs `ninja` to make sure container build contexts are prepared.
1. Makes sure the base container image exists for Compose applications.

The build contexts for the containers should reflect any changes to
source code files, which implies that the build script (`Makefile` or
`ninja.build`) should be updated when the list of source files changes.
This is much easier to manage with Ninja.

The base container image is built by `images/builder` and providse the
toolchain for the build stage of multi-stage images.
The testnet will always have an image built by `images/boss` that
interprets the test script and instantiates clients.
Other IRC software is created by other subdirectories of `images`, using
git submodules to pull in the respective software.

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

## Code Coverage

Most IRC software images enable code coverage outputs for their
binaries.
The orchestrator in turn collects these after running test cases under
`/tests/` directory.
Simple execution of it is as `.../orchestrator .../tests/<name>`, which
will populate `.../coverage/<image>` with these contents:

- `gcno/` (as needed) with the *.gcno files and perhaps other compiler
  output, from the `build` stage of the respective image.
- `gcda/` (per test run) with the *.gcda files for a given profile run.
- `html/` with the generated HTML reports.
- `lcov.dat` contains lcov's internal representation of coverage data.
