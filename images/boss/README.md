# go-testnet

This package contains Go code that helps manage an IRC test network.
It generates one executable, `boss`.

The intended use case is a set of virtual machines (or containers) with
IRC servers running on some VMs, and clients instantiated by the `boss`
VM as it executes a script.
The set of VMs ("application") is configured and launched by some
external person or program.
`boss` also implements an ident server.

See the source code for details of the supported script syntax.
