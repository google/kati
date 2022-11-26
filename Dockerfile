FROM ubuntu:22.04

RUN apt-get update && apt-get install -y make git-core build-essential curl ninja-build python3 wget

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

# Set environment variables for Go and Make.
ENV GOROOT /goroot
ENV GOPATH /gopath
ENV PATH $GOROOT/bin:$GOPATH/bin:/make:$PATH

# Copy project code.
COPY . /src
WORKDIR /src

CMD make test -j8
