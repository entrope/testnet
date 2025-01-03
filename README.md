# What Is It?

This repository contains scripts to build a containerized IRC test
network based on a scriptable network driver and
ircu2, iauthd-c, and srvx 1.x.

## A Really Quick Start

Run `bootstrap.sh`.
Then you can run `./orchestrate/orchestrate tests/simple`
to set up and run a simple test network.
After running one or more test scripts, run `make coverage` and then
`open coverage/*/html/index.html`.

## The Longer Story

`Makefile` contains rules to build `orchestrate` and the tarballs needed
for building container images for various IRC software.
`images/builder` will generate a toolchain image that other container
images use to build their runtime stages.
`images/boss` also uses that toolchain image to compile the scriptable
network driver.

Each testnet scenario is associated with a single directory under `tests`.
The directory must contain an `irc.tmpl` file that describes the testnet.
`orchestrate` processes the Go text templates within `irc.tmpl` to
create other files:

- `compose.yaml` as the Compose application description.
- `irc.script` as the main script for the `boss` (coordinator) container.
- Config files in folders named after the virtual machine that use them.

## Script Syntax

`orchestrate` interprets commands that relate to virtual machines:

- `CIDR <ip>/<nbits>` to assign IP addresses for new clients and servers.
  The initial netmask is 10.11.12.0/24.
  Address assignment starts from `<ip>`.
  A previously used netmask can be re-used as long as the last IP
  assigned from that range is less than then new `<ip>`.
  Clients' IP assignments are communicated to `boss` through `extra_hosts`
  entries in the Compose spec, which translate to `/etc/hosts` entries.
- `CLIENT <name>[@<name>] <server>[/tls] ...` to determine which IP
  addresses to assign to the `boss` container, and to check that the
  server names are valid.
- `SERVER <name> <image>` to define the services within the Compose app.
- `SUFFIX <suffix>` to interpret `...` as a hostname suffix.

`boss` interprets commands that relate to dynamic behavior:

- `CLIENT <name>[@<name>] <server>[/tls] [<username>]` to instantiate a
  new client.
- `EXPECT [!]<client>[@<timeout>] :<regexp>` to block a client until it
  gets a line matching `<regexp>`.
  The timeout is a Go duration, defaulting to `10s`.
  If `!` is given, the script will fail when the timeout expires.∏
  Otherwise only a warning will be printed.
- `SEND [!]<client> :<text>` sends text from a client.
  If `!` is given, the client's normal rate-limiting will be skipped.
- `SUFFIX <suffix>` to interpret `...` as a hostname suffix.
- `WAIT [<client> ...]` waits for expectations from the named clients.
  If no clients are named, waits for all clients' current expectations.

Each client has a set of variables that can be expanded as `${Name}`
within `EXPECT` and `SEND` lines.
There are several predefined variables:

Variable  |  Meaning
--------- | --------
`me` | Client's current nickname
`channel` | Last channel that client joined; initially the empty string

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
`orchestrate` in turn collects these after running test cases under
`/tests/` directory.
Simple execution of it is as `.../orchestrate .../tests/<name>`, which
will populate `.../coverage/<image>` with these contents:

- `gcno/` (as needed) with the *.gcno files and perhaps other compiler
  output, from the `build` stage of the respective image.
- `gcda/` (per test run) with the *.gcda files for a given profile run.
- `html/` with the generated HTML reports.
- `lcov.dat` contains lcov's internal representation of coverage data.

## TODOs

- [ ] Provide a way to escape regexp metacharacters when expanding
  variable values for `EXPECT`'s regular expression.
- [ ] Find out why lcov reports inconsistent coverage results, such as
  below (which motivates `ignore_errors = ...,inconsistent` in lcovrc):

```text
Reading tracefile lcov.dat.
genhtml: WARNING: (inconsistent) "/Users/mdpoole/src/testnet/ircu2/ircd/s_auth.c":830: line is not hit but at least one branch on line has been evaluated.
        To skip consistency checks, see the 'check_data_consistency' section in man lcovrc(5).
        (use "genhtml --ignore-errors inconsistent,inconsistent ..." to suppress this warning)
genhtml: WARNING: (inconsistent) "/Users/mdpoole/src/testnet/ircu2/ircd/m_nick.c":301: line is hit but no branches on line have been evaluated.
```
