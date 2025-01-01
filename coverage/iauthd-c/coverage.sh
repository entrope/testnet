#! /bin/sh -e

# Make sure the .gcno files are available.
test -d gcno || ( mkdir gcno && cd gcno && tar xf ../../../packages/iauthd-c-bin.tar.bz2)

# Collect output data, or generate HTML.
GCDA=${1-gcda}
if test x${GCDA} = xhtml ; then
	genhtml -o html --config-file lcovrc lcov.dat
else
	lcov --capture --directory ${GCDA} --config-file lcovrc --output-file lcov.dat
fi
