What Is It?
-----------

This repository contains scripts to build lightweight containers for an
IRC test network based on ircu2, iauthd-c, and srvx 1.x.

A Really Quick Start
--------------------
    $ make

(Lots of output, culminating in the creation of `./packages`.)

    $ docker-compse up

(Lots of output, culminating in a bunch of Docker containers running.)

The Longer Story
----------------

The `Makefile` in this directory will build a development Docker image
and copy Alpine Linux packages out of the image.  It uses git submodules
to store copies of the source code to use; you can delete the tarballs
in `builder/*/*.tar.gz` to generate new ones from the working trees.
The generated packages contain debug symbols for easier debugging.

Once the `./packages` directory is populated, `docker-compose up` will
build the requisite images and containers, and start the containers in
a private network.

Debugging Crashes
-----------------

If you need to debug a crash inside the testnet, you will probably want
to do something like `apk add -U gdb` (as root) to install gdb in the
container(s) seeing crashes.

Linux users may also need to adjust `/proc/sys/kernel/core_pattern` to
ensure corefiles are dumped in the container.  (If `core_pattern` says
to invoke an executable, that executable will probably not exist in the
container; the easiest way is to reset it to `core` or some formatted
pattern, but be aware that this sysctl is not virtualized as of
Linux 4.7.)

Host-Side Development
---------------------

The `.gitignore` file ignores `/+*/` to support the creation of build
directories on the host system -- for example, +iauthd-c for the
iauthd-c submodule.
