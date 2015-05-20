#!/bin/sh

set -e

mk="$@"

cat <<EOF > Makefile
all: foo

foo:
	echo foo
EOF
# Pretend to be a very old Makefile.
touch -t 197101010000 Makefile

"$@" | tee /tmp/log 2>&1

if [ -e .kati_cache.Makefile ]; then
  if ! grep '\*kati\*: Cache not found' /tmp/log; then
    echo 'Cache unexpectedly found'
  fi
fi

"$@" | tee /tmp/log 2>&1

if [ -e .kati_cache.Makefile ]; then
  if ! grep '\*kati\*: Cache found' /tmp/log; then
    echo 'Cache unexpectedly not found'
  fi
fi

cat <<EOF >> Makefile
	echo bar
EOF

"$@" | tee /tmp/log 2>&1

if [ -e .kati_cache.Makefile ]; then
  if ! grep '\*kati\*: Cache not found' /tmp/log; then
    echo 'Cache unexpectedly found'
  fi
fi
