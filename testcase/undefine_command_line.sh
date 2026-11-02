#!/bin/bash

set -u

mk="$@"

cat > Makefile <<'EOF'
command_line_var = from-file
undefine command_line_var
$(info after_undefine=$(origin command_line_var))
override undefine command_line_var
$(info after_override=$(origin command_line_var))
all:
	@:
EOF

if echo "$mk" | grep -qv "kati"; then
  make all command_line_var=from-command-line
else
  $mk all command_line_var=from-command-line
fi
