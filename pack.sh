#!/bin/sh

set -e

make kati

rm -fr out/kati
mkdir out/kati
git archive --prefix src/ master | tar -C out/kati -xvf -

cd out/kati
rm src/repo/android.tgz
cp ../../m2n ../../kati .
cd ..
tar -cvzf ../kati.tgz kati
