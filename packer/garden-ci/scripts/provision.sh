set -e -x

# install build dependencies
# - graphviz is for rendering heap w/ pprof

apt-get update && \
apt-get -y install \
  build-essential \
  curl \
  git \
  graphviz \
  htop \
  libpython-dev \
  lsof \
  psmisc \
  python \
  strace \
  wget \
  iptables \
  aufs-tools \
  quota \
  ulogd

# install go1.4
wget -qO- https://storage.googleapis.com/golang/go1.4.2.linux-amd64.tar.gz | tar -C /usr/local -xzf -

#Set up $GOPATH and add go executables to $PATH
cat > /etc/profile.d/go_env.sh <<\EOF
export GOPATH=$HOME/go
export PATH=$GOPATH/bin:/usr/local/go/bin:$PATH
EOF
chmod +x /etc/profile.d/go_env.sh

export GOPATH=$HOME/go
export PATH=/usr/local/go/bin:$PATH

# install Mercurial (for hg go dependencies)
wget -qO- http://mercurial.selenic.com/release/mercurial-2.9.2.tar.gz | tar -C /tmp -xzf - && \
  cd /tmp/mercurial-2.9.2 && \
  sudo python setup.py install && \
  rm -rf /tmp/mercurial-2.9.2
cd -

# install common CI dependencies
go get \
  github.com/dustin/goveralls \
  golang.org/x/tools/cmd/cover

# create dir for rootfses to upload to
mkdir -p /opt/warden
chmod 0777 /opt/warden
