# The Test Harness

## Introduction

The test harness takes a directory containing a test script, creates a
[Compose](https://github.com/compose-spec/compose-spec/blob/main/spec.md)
file able to execute the script, executes it, checks the output and
extracts coverage data for servers.
The test script contains configuration snippets for each IRC server
and service in the test network; the expanded files are written into
files in the test script's directory.

If a compose is running locally, and irc-1/irc.tmpl is a test script,
you can use the test driver like this:

```text
  orchestrate -tool=podman irc-1
```

`orchestrate` scans the script to find each virtual host within it, and
(by default) constructs a Compose file that can execute the script when
started.
A main container is defined to be the controller.
Other containers in the pod execute either as servers or clients within
the IRC network.
A single client container may create multiple clients, which share the
container's IP address when connecting to servers; each client container
acts a lot like IRC bouncers, run by the controller.
A server container may only execute a single server.

When the pod exits, `orchestrate` will copy the session log from the
controller and and code-coverage files (if any) from the servers.
The coverage report will be cumulative across test scripts, until you
delete lcov's working data.

## Test Script Introduction

The test script is a
[Go text template](https://golang.org/pkg/text/template/)
with named templates for each server's configuration file.
The top-level template is executed to generate the test script.
Each server gets a configuration file that is generate from a named
template within the test script, identified by the server's name.

```text
SUFFIX example.org
IRCD irc-1...
HUB svc-1...
CLIENT user1 irc-1
:user1 USER user1 12 _ :Some scripted user
:user1 NICK user1

{{define "irc-1...:/home/ircd/etc/ircd.conf"}}
General { name = "{{ .Me }}"; }
{{end}}
```

The test script normally creates at least one server and one client.
After that, the client registers.
The server configuration templates may be anywhere in the file, but it
is tidier to have them at the end, after the test script body.

Note: The test harness will automatically send `PONG` responses to a
`PING` message *unless* the `PING` is captured by an `EXPECT`.  (To be
precise, an uncaptured `PING` sets a flag for the client, and that flag
causes the client to send a `PONG <token>` when the client either
executes a low-priority command or compares an incoming line to an
`EXPECT`.)

Note that each server's configuration template can use the following
keys of a map that is passed to the server's configuration template:

Key | Content
--- | -------
`Me` | Server's short name
`IP` | Map of server names to their IP addresses (as strings)

## Script Reference

### Line Parsing

Each line is processed in order.
First, leading whitespace is removed, and blank lines are skipped.
Lines starting with `#` are skipped as comments.
Lines starting with `:` are treated as if they instead start with `SEND `.

Each non-comment, non-blank line is parsed as a command name, optionally
followed by whitespace and then optional arguments.

Each command that is associated with a client is added to a queue for
that client.

### Client, Server, and Variable Names

Script names for clients, servers, and variables must start with an
ASCII letter or underscore, and may contain any number of ASCII letters,
numbers or underscores after that.
All names are case-sensitive.
(The restriction of "ASCII" means in ASCII, and excludes the RFC1459
extra "letters" with code points that correspond to ASCII punctuation:
`[\]{|}`.)

Server names, including services and hubs, may be written to have `...`
at the end.
In this case, the last two dots are replaced with the suffix given by
the SUFFIX command.
In the example above, the ircd name becomes `irc-1.example.org`.

The following variables have pre-defined meanings:

Variable  |  Meaning
--------- | --------
`_` | Empty string on read; immutable (see `EXPECT`)
`me` | Client's current nickname
`channel` | Last channel that client joined; initially the empty string

Each client has its own set of variable values.
They are used by writing `${variable}` within a message to send or an
`EXPECT` regular expression.
Another client's variables may be read by writing `${variable@client}`.
It is a fatal error to use a variable before its value is set.

### Fatal and Other Errors

Variable references and unmet expectations (the `EXPECT` command) may
cause errors.
Some errors are fatal, which means the script fails immediately when the
error is detected.
Non-fatal errors are reported when they are detected, but the script
continues executing.

Using an undefined variable is fatal.

An unmet expectation where the client specifier included `!` is fatal.

### IRCD and HUB Command Syntax

The syntax for the IRCD or HUB command is:

```text
IRCD <name>
HUB <name>
```

These commands look for a template within the script named
`&lt;name>.conf`, and creates a Compose
[config](https://github.com/compose-spec/compose-spec/blob/main/08-configs.md)
with that name to pass to the corresponding container.
Then the command launches the container.

The only difference between `IRCD` and `HUB` is that an `IRCD` connects
to both the client-side and server-only networks, whereas a `HUB` only
connects to the server-only network (and is not reachable by clients).

### CLIENT Command Syntax

The syntax for the CLIENT command is:

```text
CLIENT <name>[@<client>] <server> [<username>]
```

The `name` of the new client is used for issuing later commands.
If `@client` is present, the new client will run from the same
container as `client`; otherwise, it will get a new container.
The new client connects to `server`.
If `username` is present, an
[ident](https://tools.ietf.org/html/rfc1413)
server will run on the client's container, and answer with the specified
username for this client's connection.

If more than one client is running from the same container, and only
some have `username` specified, the ident server will return an
`ERROR : NO-USER` response.
If no clients on a container have a `username`, no ident server will
run on that container.

### EXPECT Command Syntax

The syntax for the EXPECT command is:

```text
EXPECT <client>[@<timeout>][!] [<var1>,<var2>,...] :<regexp>
```

This command looks for a line received by `client` that matches `text`.
If `@timeout` is present, it specifies the number of seconds to wait for
a matching line; the default is 10 seconds.
The timeout may include a fractional part, as in 3.14159, although
network traffic timing is somewhat noisy, so fine-grained timeouts are
seldom useful.
If `!` is present, a timeout is fatal.
If a list of variable names is given, they must correspond to capturing
groups in the regular expression, and they are assigned when a line
matches that regexp.  The `_` variable name discards the corresponding
captured text.
The `regexp` may contain one or more variable names; these are expanded
before the regular expression is parsed.

For example, `EXPECT user1 _,token :(.+ )?PING (.*)` will match both
`:server PING 12345` and `PING 12345`, discarding the `:server` in the
first case (the `_` variable is read-only), and saving the text `12345`
into the variable `${token}` in both cases.

### SEND Command Syntax

The syntax for the SEND command is:

```text
SEND [!]<client> <text...>
```

This command sends `text...` from the named `client` to its server.

By default, the test controller will rate-limit the number of lines it
sends through a client (to approximately one every two seconds, with up
to five lines of burst allowed, matching IRC's historic rate limit).

If the client's nickname is preceded by `!`, then this `SEND` command
will not apply that rate-limiting, and is a high-priority command (it
will not trigger an automatic `PONG` for the client).

### WAIT Command Syntax

The syntax for the WAIT command is:

```text
WAIT [<client1> ...]
```

This command waits until the specified client(s) have no queued commands.
If no client is specified, this command waits until all queued commands
have been executed.

`WAIT` is useful for sequencing events, especially after `EXPECT`.

## TODOs

- [ ] Provide a way to escape regexp metacharacters when expanding
  variable values for `EXPECT`'s regular expression.
- [ ] Decide whether clients or servers should be able to define their
  own IP addresses.  This implies being able to configure an overlay
  network for the swarm.
