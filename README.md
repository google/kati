kati
====

[![Build and Test](https://github.com/google/kati/workflows/Build%20and%20Test/badge.svg)](https://github.com/google/kati/actions)

kati is an experimental GNU make clone.
The main goal of this tool is to speed-up incremental build of Android.

Currently, kati does not offer a faster build by itself. It instead converts
your Makefile to a ninja file.

How to use for Android
----------------------

For Android-N+, ckati and ninja is used automatically. There is a prebuilt
checked in under prebuilts/build-tools that is used.

All Android's build commands (m, mmm, mmma, etc.) should just work.

How to use for Android (deprecated -- only for Android M or earlier)
----------------------

Set up kati:

    % cd ~/src
    % git clone https://github.com/google/kati
    % cd kati
    % make

Build Android:

    % cd <android-directory>
    % source build/envsetup.sh
    % lunch <your-choice>
    % ~/src/kati/m2n --kati_stats
    % ./ninja.sh

You need ninja in your $PATH.

More usage examples (deprecated way)
-------------------

### "make clean"

    % ./ninja.sh -t clean

Note ./ninja.sh passes all parameters to ninja.

### Build a specific target

For example, the following is equivalent to "make cts":

    % ./ninja.sh cts

Or, if you know the path you want, you can do:

    % ./ninja.sh out/host/linux-x86/bin/adb
