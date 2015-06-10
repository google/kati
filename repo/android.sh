#!/bin/sh
#
# Copyright 2015 Google Inc. All rights reserved
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

dir=$(realpath $(dirname $0))

export ANDROID_BUILD_PATHS=${dir}/android/out/host/linux-x86/bin:${dir}/android/prebuilts/gcc/linux-x86/arm/arm-linux-androideabi-4.9/bin:${dir}/android/prebuilts/gcc/linux-x86/:${dir}/android/development/scripts:${dir}/android/prebuilts/devtools/tools:${dir}/android/prebuilts/android-emulator/linux-x86_64:
export ANDROID_BUILD_TOP=${dir}/android
export ANDROID_DEV_SCRIPTS=${dir}/android/development/scripts:${dir}/android/prebuilts/devtools/tools
export ANDROID_EMULATOR_PREBUILTS=${dir}/android/prebuilts/android-emulator/linux-x86_64
export ANDROID_HOST_OUT=${dir}/android/out/host/linux-x86
export ANDROID_JAVA_TOOLCHAIN=/usr/lib/jvm/java-7-openjdk-amd64/bin
export ANDROID_PRE_BUILD_PATHS=/usr/lib/jvm/java-7-openjdk-amd64/bin:
export ANDROID_PRODUCT_OUT=${dir}/android/out/target/product/generic
export ANDROID_SET_JAVA_HOME=true
export ANDROID_TOOLCHAIN=${dir}/android/prebuilts/gcc/linux-x86/arm/arm-linux-androideabi-4.9/bin
export ANDROID_TOOLCHAIN_2ND_ARCH=${dir}/android/prebuilts/gcc/linux-x86/
export ASAN_OPTIONS=detect_leaks=0
export BUILD_ENV_SEQUENCE_NUMBER=10
export GCC_COLORS="error=01;31:warning=01;35:note=01;36:caret=01;32:locus=01:quote=01"
export JAVA_HOME=/usr/lib/jvm/java-7-openjdk-amd64
export OUT=${dir}/android/out/target/product/generic
export PATH=/usr/lib/jvm/java-7-openjdk-amd64/bin:${dir}/android/out/host/linux-x86/bin:${dir}/android/prebuilts/gcc/linux-x86/arm/arm-linux-androideabi-4.9/bin:${dir}/android/prebuilts/gcc/linux-x86/:${dir}/android/development/scripts:${dir}/android/prebuilts/devtools/tools:${dir}/android/prebuilts/android-emulator/linux-x86_64:${dir}/..:${PATH}
export TARGET_BUILD_APPS=
export TARGET_BUILD_TYPE=release
export TARGET_BUILD_VARIANT=eng
export TARGET_GCC_VERSION=4.9
export TARGET_PRODUCT=aosp_arm

cd ${dir}/android

"$@"
