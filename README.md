kati
====

kati is an experimental GNU make clone.
The main goal of this tool is to speed-up incremental build of Android.

How to use for Android
----------------------

Currently, kati does not offer a faster build by itself. It instead converts
your Makefile to a ninja file.

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

More usage examples
-------------------

### "make clean"

    % ./ninja.sh -t clean

Note ./ninja.sh passes all parameters to ninja.

### Build a specific target

For example, the following is equivalent to "make cts":

    % ~/src/kati/m2n cts
    % ./ninja-cts.sh

Or, if your target is built by "make", you can specify the target of ninja.

    % ./ninja.sh out/host/linux-x86/bin/adb

### Specify the number of default jobs used by ninja

    % ~/src/kati/m2n -j10
    % ./ninja.sh

Or

    % ./ninja.sh -j10

Note the latter kills the parallelism of goma.
