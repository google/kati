set -e

mk="$@"

cat <<EOF > Makefile
test: test2
	echo PASS
include myfile.mk
EOF

mkdir -p test_dir
echo -e "test2:\n\techo \$@" > test_dir/myfile.mk
${mk} --include-dir=test_dir 2> /dev/null
