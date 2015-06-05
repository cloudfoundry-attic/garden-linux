set -e
set -x

apt-get update
apt-get install -y asciidoc xmlto --no-install-recommends
apt-get install -y pkg-config autoconf
apt-get build-dep -y btrfs-tools
mkdir -p $HOME/btrfs
cd $HOME/btrfs
git clone git://git.kernel.org/pub/scm/linux/kernel/git/kdave/btrfs-progs.git
cd btrfs-progs
./autogen.sh
./configure
make
make install
