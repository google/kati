#!/bin/sh

set -e

cd $(dirname $0)

rm -fr strace
git clone https://github.com/shinh/strace.git
cd strace

./bootstrap
./configure
make -j20
