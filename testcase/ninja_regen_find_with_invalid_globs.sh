set -euo pipefail

mk="$@"

# This tests behavior for a bug in the find emulator:
# The *.xml and .* are not expanded despite not being quoted. So if the find emulator
# weren't used, the find would fail with invalid arguments. Probably should fix this
# in the future

mkdir -p bootable/recovery/tools/recovery_l10n/res/values/
mkdir -p bootable/recovery/tools/recovery_l10n/res/layout/
touch bootable/recovery/tools/recovery_l10n/res/values/strings.xml
touch bootable/recovery/tools/recovery_l10n/res/layout/main.xml
touch .cursorignore
touch .gemini
touch .repo

if echo "${mk}" | grep kati > /dev/null; then
  mk="${mk} --use_find_emulator"
fi

cat <<EOF > Makefile
resource_dir := bootable/recovery/tools/recovery_l10n/res/
resource_dir_deps := \$(sort \$(shell find \$(resource_dir) -name *.xml -not -name .*))
all: \$(resource_dir_deps)
	@echo Hello, world!
EOF

${mk} 2>stderr_log
if [ -e ninja.sh ]; then
  ./ninja.sh
fi

${mk} 2>stderr_log2
if [ -e ninja.sh ]; then
  if grep regenerating stderr_log2 > /dev/null; then
    echo 'Should not regenerate'
  fi
  ./ninja.sh
fi
