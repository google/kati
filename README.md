kati
====

[![Build Status](https://travis-ci.org/google/kati.svg?branch=master)](http://travis-ci.org/google/kati)

kati is an experimental GNU make clone.
The main goal of this tool is to speed-up incremental build of Android.

Currently, kati does not offer a faster build by itself. It instead converts
your Makefile to a ninja file.

How to use for Android
----------------------

Now AOSP has kati and ninja, so all you have to do is

    % export USE_NINJA=true

All Android's build commands (m, mmm, mmma, etc.) should just work.

How to use for Android (deprecated way)
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
    % ~/src/kati/m2n --kati_stats  # Use --goma if you are a Googler.
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
