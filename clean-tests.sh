#! /bin/sh -e

rm -f tests/*/compose.yaml tests/*/irc.script
for dir in tests/*/* ; do
  if test -d $dir ; then
    rm -r $dir
  fi
done
