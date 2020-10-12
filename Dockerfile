FROM ubuntu:18.04

RUN apt-get update && apt-get install -y make git-core build-essential curl ninja-build python

# Install Go
RUN \
  mkdir -p /goroot && \
  curl https://storage.googleapis.com/golang/go1.14.9.linux-amd64.tar.gz | tar xvzf - -C /goroot --strip-components=1

# Set environment variables for Go.
ENV GOROOT /goroot
ENV GOPATH /gopath
ENV PATH $GOROOT/bin:$GOPATH/bin:$PATH

# Copy project code.
COPY . /

ENTRYPOINT make test
