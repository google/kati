package main

import "testing"

func TestStripShellComment(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
	}{
		{
			in:   `foo`,
			want: `foo`,
		},
		{
			in:   `foo # bar`,
			want: `foo `,
		},
		{
			in:   `foo '# bar'`,
			want: `foo '# bar'`,
		},
		{
			in:   `foo '\'# bar'`,
			want: `foo '\'`,
		},
		{
			in:   `foo "# bar"`,
			want: `foo "# bar"`,
		},
		{
			in:   `foo "\"# bar"`,
			want: `foo "\"# bar"`,
		},
		{
			in:   `foo "\\"# bar"`,
			want: `foo "\\"`,
		},
		{
			in:   "foo `# bar`",
			want: "foo `# bar`",
		},
		{
			in:   "foo `\\`# bar`",
			want: "foo `\\`# bar`",
		},
		{
			in:   "foo `\\\\`# bar`",
			want: "foo `\\\\`",
		},
	} {
		got := stripShellComment(tc.in)
		if got != tc.want {
			t.Errorf(`stripShellComment(%q)=%q, want %q`, tc.in, got, tc.want)
		}
	}
}

func TestGetDepFile(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		err  bool
	}{
		{
			in:   `g++ -c fat.cc -o fat.o`,
			want: ``,
		},
		{
			in:  `g++ -c fat.cc -MD`,
			err: true,
		},
		{
			in:  `g++ -c fat.cc -MD -o fat.o -o fat.o`,
			err: true,
		},
		{
			in:   `g++ -c fat.cc -MD -o fat.o`,
			want: `fat.d`,
		},
		{
			in:   `g++ -c fat.cc -MD -o fat`,
			want: `fat.d`,
		},
		{
			in:   `g++ -c fat.cc -MD -MF foo.d -o fat.o`,
			want: `foo.d`,
		},
		{
			in:   `g++ -c fat.cc -MD -o fat.o -MF foo.d`,
			want: `foo.d`,
		},
		// A real example from maloader.
		{
			in:   `g++ -g -Iinclude -Wall -MMD -fno-omit-frame-pointer -O -m64 -W -Werror   -c -o fat.o fat.cc`,
			want: `fat.d`,
		},
		// A real example from Android.
		{
			in:   `mkdir -p out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/ && echo "host C++: llvm-rs-cc <= frameworks/compile/slang/llvm-rs-cc.cpp" && prebuilts/clang/linux-x86/host/3.6/bin/clang++ -I external/llvm -I external/llvm/include -I external/llvm/host/include -I external/clang/include -I external/clang/lib/CodeGen -I frameworks/compile/libbcc/include -I out/host/linux-x86/gen/EXECUTABLES/llvm-rs-cc_intermediates/include -I external/libcxx/include -I frameworks/compile/slang -I out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates -I out/host/linux-x86/gen/EXECUTABLES/llvm-rs-cc_intermediates -I libnativehelper/include/nativehelper $(cat out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/import_includes) -isystem system/core/include -isystem hardware/libhardware/include -isystem hardware/libhardware_legacy/include -isystem hardware/ril/include -isystem libnativehelper/include -isystem frameworks/native/include -isystem frameworks/native/opengl/include -isystem frameworks/av/include -isystem frameworks/base/include -isystem tools/include -isystem out/host/linux-x86/obj/include -c    -fno-exceptions -Wno-multichar -m64 -Wa,--noexecstack -fPIC -no-canonical-prefixes -include build/core/combo/include/arch/linux-x86/AndroidConfig.h -U_FORTIFY_SOURCE -D_FORTIFY_SOURCE=0 -D__STDC_FORMAT_MACROS -D__STDC_CONSTANT_MACROS -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -O2 -g -fno-strict-aliasing -DNDEBUG -UDEBUG  -D__compiler_offsetof=__builtin_offsetof -Werror=int-conversion -Wno-unused-command-line-argument   --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/    --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/ --sysroot=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//sysroot -target x86_64-linux-gnu   -DANDROID -fmessage-length=0 -W -Wall -Wno-unused -Winit-self -Wpointer-arith -Wsign-promo -std=gnu++11 -DNDEBUG -UDEBUG  -Wno-inconsistent-missing-override   --gcc-toolchain=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8/ --sysroot=prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//sysroot -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8 -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8/x86_64-linux -isystem prebuilts/gcc/linux-x86/host/x86_64-linux-glibc2.15-4.8//x86_64-linux/include/c++/4.8/backward -target x86_64-linux-gnu    -pedantic -Wcast-qual -Wno-long-long -Wno-sign-promo -Wall -Wno-unused-parameter -Wno-return-type -Werror -std=c++11 -O0 -DTARGET_BUILD_VARIANT=eng -DRS_VERSION=23 -D_GNU_SOURCE -D__STDC_LIMIT_MACROS -O2 -fomit-frame-pointer -Wall -W -Wno-unused-parameter -Wwrite-strings -Dsprintf=sprintf -pedantic -Wcast-qual -Wno-long-long -Wno-sign-promo -Wall -Wno-unused-parameter -Wno-return-type -Werror -std=c++11 -O0 -DTARGET_BUILD_VARIANT=eng -DRS_VERSION=23 -fno-exceptions -fpie -D_USING_LIBCXX   -Wno-sign-promo -fno-rtti -Woverloaded-virtual -Wno-sign-promo -std=c++11 -nostdinc++  -MD -MF out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d -o out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.o frameworks/compile/slang/llvm-rs-cc.cpp && cp out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P; sed -e 's/#.*//' -e 's/^[^:]*: *//' -e 's/ *\\$//' -e '/^$/ d' -e 's/$/ :/' < out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d >> out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P; rm -f out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.d`,
			want: `out/host/linux-x86/obj/EXECUTABLES/llvm-rs-cc_intermediates/llvm-rs-cc.P`,
		},
	} {
		got, err := getDepfile(tc.in)
		if got != tc.want {
			t.Errorf(`stripShellComment(%q)=%q, want %q`, tc.in, got, tc.want)
		}
		if tc.err && err == nil {
			t.Errorf(`stripShellComment(%q) unexpectedly has no error`, tc.in)
		} else if !tc.err && err != nil {
			t.Errorf(`stripShellComment(%q) has an error: %q`, err)
		}
	}
}
