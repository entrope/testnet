#! /bin/sh -e

cd ~/irc
PACKAGER="coder-com <coder-com@undernet.org>"
export PACKAGER
abuild-keygen -a -i < /dev/null
mkdir ${HOME}/packages
grep coder-com /etc/group > ${HOME}/packages/group
grep coder-com /etc/passwd > ${HOME}/packages/passwd

for subdir in `find * -maxdepth 0 -type d` ; do
    cd ${subdir}
    abuild checksum
    abuild -r -s .
    cd -
done

source ${HOME}/.abuild/abuild.conf
cp ${PACKAGER_PRIVKEY}.pub ${HOME}/packages/
