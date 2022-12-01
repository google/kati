// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build ignore

#include "ninja.h"

#include <assert.h>

#include "log.h"
#include "testutil.h"

namespace {

std::string GetDepfile(std::string cmd, std::string* new_cmd) {
  new_cmd->clear();
  std::string r;
  if (GetDepfileFromCommand(&cmd, &r)) {
    *new_cmd = cmd;
    return r;
  }
  return "";
}

void TestGetDepfile() {
  std::string new_cmd;
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -MD ", &new_cmd), "");
  assert(g_last_error);
  delete g_last_error;
  g_last_error = NULL;

  // clang-format off
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -o fat.o", &new_cmd), "");
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -MD -o fat.o -o fuga.o", &new_cmd), "fuga.d.tmp");
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -MD -o fat.o", &new_cmd), "fat.d.tmp");
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -MD -o fat", &new_cmd), "fat.d.tmp");
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -MD -MF foo.d -o fat.o", &new_cmd), "foo.d.tmp");
  ASSERT_EQ(GetDepfile("g++ -c fat.cc -MD -o fat.o -MF foo.d", &new_cmd), "foo.d.tmp");
  // A real example from maloader.
  ASSERT_EQ(GetDepfile("g++ -g -Iinclude -Wall -MMD -fno-omit-frame-pointer -O -m64 -W -Werror   -c -o fat.o fat.cc", &new_cmd), "fat.d.tmp");
  // A real example from Android.
  ASSERT_EQ(GetDepfile("mkdir -p out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/ && echo \"host C++: llvm-rs-cc <= frameworks/compile/slang/llvm-rs-cc.cpp\" && prebuilts/clang/linux-x86/host/3.6/bin/clang++ -I external/llvm -I external/llvm/include -I external/llvm/host/include -I external/clang/include -I external/clang/lib/CodeGen -I frameworks/compile/libbcc/include -I out/host/linux-x86/gen/EXECUTABLES/llvm-rs-cc_intermediates/include -I external/libcxx/include -I frameworks/compile/slang -I out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates -I out/host/linux-x86/gen/EXECUTABLES/llvm-rs-cc_intermediates -I libnativehelper/include/nativehelper $(cat out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/import_includes) -isystem system/core/include -isystem hardware/libhardware/include -isystem hardware/libhardware_legacy/include -isystem hardware/ril/include -isystem libnativehelper/include -isystem frameworks/native/include -isystem frameworks/native/opengl/include -isystem frameworks/av/include -isystem frameworks/base/include -isystem tools/include -isystem out/host/linux-x86/obj/include -c    -fno-exceptions -Wno-multichar -m64 -Wa,--noexecstack -fPIC -no-canonical-prefixes -include build/core/combo/include/arch/linux-x86/AndroidConfig.h -U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0 -D__STDC_FORMAT_MACROS -D__STDC_CONSTANT_MACROS -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -O2 -g -fno-strict-aliasing -DNDEBUG -UDEBUG  -D__compiler_offsetof=__builtin_offsetof -Werror=int-conversion -Wno-unused-command-line-argument   --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/    --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/ --sysroot=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//sysroot -target x86_64-linux-gnu   -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -Wsign-promo -std=gnu++11 -DNDEBUG -UDEBUG  -Wno-inconsistent-missing-override   --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/ --sysroot=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//sysroot -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8 -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8/x86_64-linux -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8/backward -target x86_64-linux-gnu    -pedantic -Wcast-qual -Wno-long-long -Wno-sign-promo -Wall -Wno-unused-parameter -Wno-return-type -Werror -std=c++11 -O0 -DTARGET_BUILD_VARIANT=eng -DRS_VERSION=23 -D_GNU_SOURCE -D__STDC_LIMIT_MACROS -O2 -fomit-frame-pointer -Wall -W -Wno-unused-parameter -Wwrite-strings -Dsprintf=sprintf -pedantic -Wcast-qual -Wno-long-long -Wno-sign-promo -Wall -Wno-unused-parameter -Wno-return-type -Werror -std=c++11 -O0 -DTARGET_BUILD_VARIANT=eng -DRS_VERSION=23 -fno-exceptions -fpie -D_USING_LIBCXX   -Wno-sign-promo -fno-rtti -Woverloaded-virtual -Wno-sign-promo -std=c++11 -nostdinc++  -MD -MF out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d -o out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.o frameworks/compile/slang/llvm-rs-cc.cpp && cp out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P; sed -e 's/#.*//' -e 's/^[^:]*: *//' -e 's/ *\\$//' -e '/^$/ d' -e 's/$/ :/' < out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d >> out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P; rm -f out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d", &new_cmd), "out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d");
  ASSERT_EQ(new_cmd, "mkdir -p out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/ && echo \"host C++: llvm-rs-cc <= frameworks/compile/slang/llvm-rs-cc.cpp\" && prebuilts/clang/linux-x86/host/3.6/bin/clang++ -I external/llvm -I external/llvm/include -I external/llvm/host/include -I external/clang/include -I external/clang/lib/CodeGen -I frameworks/compile/libbcc/include -I out/host/linux-x86/gen/EXECUTABLES/llvm-rs-cc_intermediates/include -I external/libcxx/include -I frameworks/compile/slang -I out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates -I out/host/linux-x86/gen/EXECUTABLES/llvm-rs-cc_intermediates -I libnativehelper/include/nativehelper $(cat out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/import_includes) -isystem system/core/include -isystem hardware/libhardware/include -isystem hardware/libhardware_legacy/include -isystem hardware/ril/include -isystem libnativehelper/include -isystem frameworks/native/include -isystem frameworks/native/opengl/include -isystem frameworks/av/include -isystem frameworks/base/include -isystem tools/include -isystem out/host/linux-x86/obj/include -c    -fno-exceptions -Wno-multichar -m64 -Wa,--noexecstack -fPIC -no-canonical-prefixes -include build/core/combo/include/arch/linux-x86/AndroidConfig.h -U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0 -D__STDC_FORMAT_MACROS -D__STDC_CONSTANT_MACROS -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -O2 -g -fno-strict-aliasing -DNDEBUG -UDEBUG  -D__compiler_offsetof=__builtin_offsetof -Werror=int-conversion -Wno-unused-command-line-argument   --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/    --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/ --sysroot=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//sysroot -target x86_64-linux-gnu   -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -Wsign-promo -std=gnu++11 -DNDEBUG -UDEBUG  -Wno-inconsistent-missing-override   --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/ --sysroot=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//sysroot -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8 -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8/x86_64-linux -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8/backward -target x86_64-linux-gnu    -pedantic -Wcast-qual -Wno-long-long -Wno-sign-promo -Wall -Wno-unused-parameter -Wno-return-type -Werror -std=c++11 -O0 -DTARGET_BUILD_VARIANT=eng -DRS_VERSION=23 -D_GNU_SOURCE -D__STDC_LIMIT_MACROS -O2 -fomit-frame-pointer -Wall -W -Wno-unused-parameter -Wwrite-strings -Dsprintf=sprintf -pedantic -Wcast-qual -Wno-long-long -Wno-sign-promo -Wall -Wno-unused-parameter -Wno-return-type -Werror -std=c++11 -O0 -DTARGET_BUILD_VARIANT=eng -DRS_VERSION=23 -fno-exceptions -fpie -D_USING_LIBCXX   -Wno-sign-promo -fno-rtti -Woverloaded-virtual -Wno-sign-promo -std=c++11 -nostdinc++  -MD -MF out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d -o out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.o frameworks/compile/slang/llvm-rs-cc.cpp && cp out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P; sed -e 's/#.*//' -e 's/^[^:]*: *//' -e 's/ *\\$//' -e '/^$/ d' -e 's/$/ :/' < out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d >> out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P");
  ASSERT_EQ(GetDepfile("echo \"target asm: libsonivox <= external/sonivox/arm-wt-22k/lib_src/ARM-E_filter_gnu.s\" && mkdir -p out/target/product/generic/obj/SHARED_LIBRARIES/libsonivox_intermediates/lib_src/ && prebuilts/gcc/linux-x86/arm/arm-linux-androideabi-4.9/bin/arm-linux-androideabi-gcc -I external/sonivox/arm-wt-22k/host_src -I external/sonivox/arm-wt-22k/lib_src -I external/libcxx/include -I external/sonivox/arm-wt-22k -I out/target/product/generic/obj/SHARED_LIBRARIES/libsonivox_intermediates -I out/target/product/generic/gen/SHARED_LIBRARIES/libsonivox_intermediates -I libnativehelper/include/nativehelper $$(cat out/target/product/generic/obj/SHARED_LIBRARIES/libsonivox_intermediates/import_includes) -isystem system/core/include -isystem hardware/libhardware/include -isystem hardware/libhardware_legacy/include -isystem hardware/ril/include -isystem libnativehelper/include -isystem frameworks/native/include -isystem frameworks/native/opengl/include -isystem frameworks/av/include -isystem frameworks/base/include -isystem out/target/product/generic/obj/include -isystem bionic/libc/arch-arm/include -isystem bionic/libc/include -isystem bionic/libc/kernel/uapi -isystem bionic/libc/kernel/uapi/asm-arm -isystem bionic/libm/include -isystem bionic/libm/include/arm -c  -fno-exceptions -Wno-multichar -msoft-float -ffunction-sections -fdata-sections -funwind-tables -fstack-protector -Wa,--noexecstack -Werror=format-security -D_FORTIFY_SOURCE=2 -fno-short-enums -no-canonical-prefixes -fno-canonical-system-headers -march=armv7-a -mfloat-abi=softfp -mfpu=vfpv3-d16 -include build/core/combo/include/arch/linux-arm/AndroidConfig.h -I build/core/combo/include/arch/linux-arm/ -fno-builtin-sin -fno-strict-volatile-bitfields -Wno-psabi -mthumb-interwork -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -Werror=return-type -Werror=non-virtual-dtor -Werror=address -Werror=sequence-point -DNDEBUG -g -Wstrict-aliasing=2 -fgcse-after-reload -frerun-cse-after-loop -frename-registers -DNDEBUG -UDEBUG     -Wa,\"-I\" -Wa,\"external/sonivox/arm-wt-22k/lib_src\" -Wa,\"--defsym\" -Wa,\"SAMPLE_RATE_22050=1\" -Wa,\"--defsym\" -Wa,\"STEREO_OUTPUT=1\" -Wa,\"--defsym\" -Wa,\"FILTER_ENABLED=1\" -Wa,\"--defsym\" -Wa,\"SAMPLES_8_BIT=1\"   -D__ASSEMBLY__ -MD -MF out/target/product/generic/obj/SHARED_LIBRARIES/libsonivox_intermediates/lib_src/ARM-E_filter_gnu.d -o out/target/product/generic/obj/SHARED_LIBRARIES/libsonivox_intermediates/lib_src/ARM-E_filter_gnu.o external/sonivox/arm-wt-22k/lib_src/ARM-E_filter_gnu.s", &new_cmd), "");
  ASSERT_EQ(GetDepfile("echo \"RenderScript: Galaxy4 <= packages/wallpapers/Galaxy4/src/com/android/galaxy4/galaxy.rs\" && rm -rf out/target/common/obj/APPS/Galaxy4_intermediates/src/renderscript && mkdir -p out/target/common/obj/APPS/Galaxy4_intermediates/src/renderscript/res/raw && mkdir -p out/target/common/obj/APPS/Galaxy4_intermediates/src/renderscript/src && out/host/linux-x86/bin/llvm-rs-cc -o out/target/common/obj/APPS/Galaxy4_intermediates/src/renderscript/res/raw -p out/target/common/obj/APPS/Galaxy4_intermediates/src/renderscript/src -d out/target/common/obj/APPS/Galaxy4_intermediates/src/renderscript -a out/target/common/obj/APPS/Galaxy4_intermediates/src/RenderScript.stamp -MD -target-api 14 -Wall -Werror  -I prebuilts/sdk/renderscript/clang-include -I prebuilts/sdk/renderscript/include packages/wallpapers/Galaxy4/src/com/android/galaxy4/galaxy.rs && mkdir -p out/target/common/obj/APPS/Galaxy4_intermediates/src/ && touch out/target/common/obj/APPS/Galaxy4_intermediates/src/RenderScript.stamp", &new_cmd), "");
  ASSERT_EQ(GetDepfile("(echo \"bc: libclcore.bc <= frameworks/rs/driver/runtime/arch/generic.c\") && (mkdir -p out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/) && (prebuilts/clang/linux-x86/host/3.6/bin/clang -Iframeworks/rs/scriptc -Iexternal/clang/lib/Headers -MD -DRS_VERSION=23 -std=c99 -c -O3 -fno-builtin -emit-llvm -target armv7-none-linux-gnueabi -fsigned-char   -Iframeworks/rs/cpu_ref -DRS_DECLARE_EXPIRED_APIS -Xclang -target-feature -Xclang +long64  frameworks/rs/driver/runtime/arch/generic.c -o out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.bc) && (cp out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.d out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.P; sed -e 's/#.*//' -e 's/^[^:]*: *//' -e 's/ *\\$$//' -e '/^$$/ d' -e 's/$$/ :/' < out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.d >> out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.P; rm -f out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.d)", &new_cmd), "out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.d");
  ASSERT_EQ(new_cmd, "(echo \"bc: libclcore.bc <= frameworks/rs/driver/runtime/arch/generic.c\") && (mkdir -p out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/) && (prebuilts/clang/linux-x86/host/3.6/bin/clang -Iframeworks/rs/scriptc -Iexternal/clang/lib/Headers -MD -DRS_VERSION=23 -std=c99 -c -O3 -fno-builtin -emit-llvm -target armv7-none-linux-gnueabi -fsigned-char   -Iframeworks/rs/cpu_ref -DRS_DECLARE_EXPIRED_APIS -Xclang -target-feature -Xclang +long64  frameworks/rs/driver/runtime/arch/generic.c -o out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.bc) && (cp out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.d out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.P; sed -e 's/#.*//' -e 's/^[^:]*: *//' -e 's/ *\\$$//' -e '/^$$/ d' -e 's/$$/ :/' < out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.d >> out/target/product/generic/obj/SHARED_LIBRARIES/libclcore.bc_intermediates/arch/generic.P)");
  ASSERT_EQ(GetDepfile("gcc -c foo.P.c", &new_cmd), "");
  ASSERT_EQ(GetDepfile("gcc -MMD foo.o -o foo", &new_cmd), "");
  // TODO: Fix for automake.
  // ASSERT_EQ(GetDepfile("(/bin/sh ./libtool  --tag=CXX   --mode=compile g++ -DHAVE_CONFIG_H -I. -I./src -I./src     -Wall -Wwrite-strings -Woverloaded-virtual -Wno-sign-compare  -DNO_FRAME_POINTER  -DNDEBUG -g -O2 -MT libglog_la-logging.lo -MD -MP -MF .deps/libglog_la-logging.Tpo -c -o libglog_la-logging.lo `test -f 'src/logging.cc' || echo './'`src/logging.cc) && (mv -f .deps/libglog_la-logging.Tpo .deps/libglog_la-logging.Plo)", &new_cmd), ".deps/libglog_la-logging.Plo");
  // ASSERT_EQ(GetDepfile("(g++ -DHAVE_CONFIG_H -I. -I./src  -I./src  -pthread     -Wall -Wwrite-strings -Woverloaded-virtual -Wno-sign-compare  -DNO_FRAME_POINTER  -g -O2 -MT signalhandler_unittest-signalhandler_unittest.o -MD -MP -MF .deps/signalhandler_unittest-signalhandler_unittest.Tpo -c -o signalhandler_unittest-signalhandler_unittest.o `test -f 'src/signalhandler_unittest.cc' || echo './'`src/signalhandler_unittest.cc) && (mv -f .deps/signalhandler_unittest-signalhandler_unittest.Tpo .deps/signalhandler_unittest-signalhandler_unittest.Po)", &new_cmd), ".deps/signalhandler_unittest-signalhandler_unittest.Po");
  // clang-format on

  assert(!g_last_error);
}

static void TestGetGomaccPosForAndroidCompileCommand() {
  ASSERT_EQ(GetGomaccPosForAndroidCompileCommand(
                "prebuilts/clang/linux-x86/host/3.6/bin/clang++ -c foo.c"),
            0);
  ASSERT_EQ(GetGomaccPosForAndroidCompileCommand(
                "prebuilts/misc/linux-x86/ccache/ccache "
                "prebuilts/clang/linux-x86/host/3.6/bin/clang++ -c foo.c"),
            39);
  ASSERT_EQ(GetGomaccPosForAndroidCompileCommand("echo foo"),
            std::string::npos);
}

}  // namespace

int main() {
  g_log_no_exit = true;
  TestGetDepfile();
  TestGetGomaccPosForAndroidCompileCommand();
  assert(!g_failed);
}
