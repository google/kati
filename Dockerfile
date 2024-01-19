FROM ubuntu:22.04

RUN apt-get update && apt-get install -y make git-core build-essential curl python3 wget unzip

# Install Go
RUN \
  mkdir -p /goroot && \
  curl https://storage.googleapis.com/golang/go1.14.9.linux-amd64.tar.gz | tar xvzf - -C /goroot --strip-components=1

# Install Make, we emulate Make 4.2.1 instead of the default 4.3 currently
RUN \
  mkdir -p /make/tmp && \
  cd /make/tmp && \
  wget http://mirrors.kernel.org/ubuntu/pool/main/m/make-dfsg/make_4.2.1-1.2_amd64.deb && \
  ar xv make_4.2.1-1.2_amd64.deb && \
  tar xf data.tar.xz && \
  mv usr/bin/make ../ && \
  cd .. && \
  rm -rf tmp/

# Install ninja, we need a newer version than is in apt
RUN \
  mkdir -p /ninja && \
  cd /ninja && \
  wget https://github.com/ninja-build/ninja/releases/download/v1.11.1/ninja-linux.zip && \
  unzip ninja-linux.zip && \
  rm ninja-linux.zip

# Set environment variables for Go and Make.
ENV GOROOT /goroot
ENV GOPATH /gopath
ENV PATH $GOROOT/bin:$GOPATH/bin:/make:/ninja:$PATH

# Copy project code.
COPY . /src
WORKDIR /src

CMD make test -j8
