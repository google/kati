---
title: CKATI
section: 1
header: User Commands
footer: ckati
---

NAME
====

ckati - experimental GNU make clone

SYNOPSIS
========

**ckati** [*OPTION*]… [*TARGET*]…

DESCRIPTION
===========

*ckati* is a C++ implementation of **kati**, an experimental **make** clone.

The motivation of **kati** was to speed up Android platform build. The Android
platform's build system is built on **GNU make** and allows developers to write
build rules in a descriptive way.

*ckati* is a complete rewrite of **GNU make** from scratch, focused on speeding
up incremental builds.

*ckati* supports two modes of execution. It can directly execute the commands
specified in the *Makefile*, or it can generate a *ninja* file corresponding
to the *Makefile*.

The *ninja* generator mode is the main mode of operation, since the built-in
executor of *ckati* lacks important features for a build system like parallel
builds.

The *ninja* generator mode is not fully compatible with **GNU make** due to
a feature mismatch between **make** and **ninja**. Since **ninja** only allows
one command per a rule, when the *Makefile* has multiple commands, *ckati*
generates a rule with the commands joined with `&&`. When `$(shell ...)`
is used, *ckati* translates it into shell's `$(...)`. This works in many cases,
but doesn't when the result of `$(shell ...)` is passed to another function:

    all:
    	echo $(if $(shell echo),FAIL,PASS)

If `-\-regen` flag is specified, *ckati* checks if anything in your environment
has changed after the previous run. If the *ninja* file doesn't need to be
regenerated, it finishes quickly.

The following is checked when deciding whether the *ninja* file should be
regenerated or not:

* The command line flags passed to *ckati*
* Timestamps of the *Makefiles* used to generate the previous *ninja* file
* Environment variables used while evaluating *Makefiles*
* Results of `$(wildcard ...)`
* Results of `$(shell ...)`

*Ckati* doesn't run `$(shell date ...)` and `$(shell echo ...)` during these
checks.

*Ckati* optimises `$(shell find ...)` calls, since the Android's build system
uses a lot of them to create a list of all .java/.mk files under a directory,
and they are slow. *Ckati* has a built-in emulator of **GNU find**. The find
emulator traverse the directory tree and creates an in-memory directory tree.
When `$(shell find ...)` is used, the find emulator returns results of
**find** commands using the cached tree, giving a performance boost.

OPTIONS
=======

**-d**

:   Print debugging information.

**-\-warn**

:   Print *ckati* warnings.

**-f** *FILE*

:   Use *FILE* as a *Makefile*.

**-c**

:   Do not run anything, only perform a syntax check.

**-i**

:   Dry run mode: print the commands that would be executed, but do not execute them.

**-s**

:   Silent operation; do not print the commands as they are executed.

**-j** *JOBS*

:   Specifies the number of *JOBS* (commands) to run simultaneously.

**-\-no\_builtin\_rules**

:   Do not provide any built-in rules.

**-\-ninja**

:   Ninja generator mode: do not execute commands directly, but generate a *ninja* file.
:   By default, the ninja file is saved as `build.ninja`, and a shell script to execute
:   **ninja** is saved as `ninja.sh`. An optional suffix can be added to the file names
:   by using **-\-ninja\_suffix** option.

**-\-ninja\_dir**

:   The directory where the *ninja* file will be generated; the default is the current directory.

**-\-ninja\_suffix**

:   The *ninja* file suffix; the default is no suffix.

**-\-use\_find\_emulator**

:   Emulate `find` command calls to improve build performance.

**-\-regen**

:   Regenerate the *ninja* file only when necessary.

**-\-detect\_android\_echo**

:   Detect the use of `$(shell echo ...)` in Android build system.

**-\-detect\_depfiles**

:   Detect dependency files.

The following options can emit warnings or errors if certain *Makefile*
features are used:

**-\-werror\_overriding\_commands**

:   Fail when overriding commands for a previously defined target.

**-\-warn\_implicit\_rules**, **-\-werror\_implicit\_rules**

:   Warn or fail when implicit rules are used.

**-\-warn\_suffix\_rules**, **-\-werror\_suffix\_rules**

:   Warn or fail when suffix rules are used.

**-\-warn\_real\_to\_phony**, **-\-werror\_real\_to\_phony**

:   Warn or fail when a real target depends on a `PHONY` target.

**-\-warn\_phony\_looks\_real**, **-\-werror\_phony\_looks\_real**

:   Warn or fail when a `PHONY` target contains slashes.

**-\-werror\_writable**

:   Fail when writing to a read-only directory.

SUPPORTED MAKE FUNCTIONS
========================

Text functions:

* `subst`
* `patsubst`
* `strip`
* `findstring`
* `filter`
* `filter-out`
* `sort`
* `word`
* `wordlist`
* `words`
* `firstword`
* `lastword`

File name functions:

* `dir`
* `notdir`
* `suffix`
* `basename`
* `addsuffix`
* `addprefix`
* `join`
* `wildcard`
* `realpath`
* `abspath`

Conditional functions:

* `if`
* `or`
* `and`

Make control functions:

* `info`
* `warning`
* `error`

Miscellaneous:

* `value`
* `eval`
* `shell`
* `call`
* `foreach`
* `origin`
* `flavor`
* `file`

EXIT STATUS
===========

**ckati** exits with a status of zero if all *Makefiles* were successfully
parsed and no targets that were built failed.

SEE ALSO
========

**make**(1), **ninja**(1)

AUTHOR
======

This manual page was contributed by Andrej Shadura.
