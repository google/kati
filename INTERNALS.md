Kati internals
==============

This is an informal document about internals of kati. This document is not meant
to be a comprehensive document of kati or GNU make. This explains some random
topics which other programmers may be interested in.

Motivation
----------

The motivation of kati was to speed up Android platform build. Especially, its
incremental build time was the main focus. Android platform's build system is a
very unique system. It provides a DSL, (ab)using Turing-completeness of GNU
make. The DSL allows developers to write build rules in a descriptive way, but
the downside of is it's complicated and slow.

When we say a build system is slow, we consider "null build" and "full
build". Null build is a build which does nothing, because all output files are
already up-to-date. Full build is a build which builds everything, because there
were nothing which have been already built. Actual build in daily development is
somewhere between null build and full build.

For Android with my fairly beefy workstation, null build took ~100 secs with GNU
make. When you changed a single C file, you needed to wait ~100 secs to see if
there's a compile error. To be fair, things were not that bad. There are tools
called mm/mmm. They allow developers to build an individual module. As they
ignore dependencies between modules, they are fast. However, you need to be
somewhat experienced to use them properly. You should know which modules will be
affected by your change. It would be nicer if you can just type "make" whenever
you change something.

This is why we created a GNU make clone from scratch. There were some other
options. One option was to replace all Android.mk by files with better
format. There is actually a longer-term project for this. Kati was planned to be
a short-term project. Another option was to hack GNU make instead developing a
clone. We didn't take this option because we thought the source code of GNU make
is tricky due to historical reason. It's written in old-style C, has a lot of
ifdefs for some unknown architectures, etc.
n
Currently, kati's main mode is --ninja mode. Instead of executing build commands
by itself, kati generates build.ninja file and
[ninja](https://github.com/martine/ninja) actually runs commands. There were
some back-and-forths before kati became the current form. Some experiments
succeeded and some others failed. We even changed the language for kati. At
first, we wrote kati in Go. We naively expected we can get enough performance
with Go. I guessed at least one of the following statements are true: 1. GNU
make is not very optimized for computation heavy Makefiles, 2. Go is fast for
our purpose, or 3. we can come up with some optimization tricks for Android's
build system. As for 3, some of such optimization succeeded but it's performance
gain didn't cancel the slowness of Go.

Go's performance would be somewhat interesting topic. I didn't study the
performance difference in detail, but it seemed both our use of Go and Go
language itself were making the Go version of kati slower. As for our fault, I
think Go version has more unnecessary string allocations than C++ version
has. As for Go, it seemed GC was the main show-stopper. For example, Android's
build system defines about one million make variables, and buffers for them will
be never freed. IIRC, this kind of allocation pattern isn't good for
non-generational GC.

Go version and test cases were written by ukai and me, and C++ rewrite was done
mostly by me. The rest of this document is mostly about the C++ version.

Overall architecture
--------------------

Kati consists of the following components:

* Parser
* Evaluator
* Dependency builder
* Executor
* Ninja generator

A Makefile has some statements which consist of zero or more expressions. There
are two parsers and two evaluators - one for statements and the other for
expressions.

Most of users of GNU make may not care about the evaluator much. However, GNU
make's evaluator is very powerful and is Turing-complete. For Android's null
build, most time is spent in this phase. Other tasks, such as building
dependency graphs and calling stat function for build targets, are not the
bottleneck. This would be a very Android specific characteristics. Android's
build system uses a lot of GNU make black-magics.

The evaluator outputs a list of build rules and a variable table. The dependency
builder creates a dependency graph from the list of build rules. Note this step
doesn't use the variable table.

Then either executor or ninja generator will be used. Either way, kati runs its
evaluator again for command lines. The variable table is necessary for this
step.

We'll look at each components closely. GNU make is a somewhat different language
from modern languages. Let's see.

Parser for statements
---------------------

I'm not 100% sure, but I think GNU make parses and evaluates Makefiles
simultaneously, but kati has two phases for parsing and evaluation. The reason
of this design is for performance. For Android build, kati (or GNU make) needs
to read ~3k files ~50k times. The file which is read most often is read ~5k
times. It's waste of time to parse such files again and again. Kati can re-use
parsed results when a Makefile is read second time. If we stop caching the
parsed results, kati will be two times slower for Android's build. Caching
parsed statements is done in *file_cache.cc*.

The statement parser is defined in *parser.cc*. In kati, there are four kinds of
statements:

* Rules
* Assignments
* Commands
* Make directives

They are defined in *stmt.h*. Here are examples of these statements:

    VAR := yay!      # An assignment
    all:             # A rule
    	echo $(VAR)  # A command
    include xxx.mk   # A make directive (include)

In addition to include directive, there are ifeq/ifneq/ifdef/ifndef directives
and export/unexport directives. Kati internally uses "parse error statement". As
GNU make doesn't show parse errors in branches which are not taken, we need to
delay parse errors to evaluation time.

### Context dependent parser

A tricky point of parsing make statements is that the parsing depends on the
context of the evaluation. See the following Makefile chunk for example:

    $(VAR)
    	X=hoge echo $${X}

You cannot tell whether the second line is a command or an assignment until
*$(VAR)* is evaluated. If *$(VAR)* is a rule statement, the second line is a
command and an assignment otherwise. If the previous line is

    VAR := target:

the second line will turn out to be a command.

For some reason, GNU make expands expressions before it decides the type of
a statement only for rules. Storing assignments or directives in a variable
won't work as assignments or directives. For example

    ASSIGN := A=B
    $(ASSIGN):

doesn't assign *B:* to *A*, but defines a build rule whose target is *A=B*.

When a line start with a tab character is ambiguous and kati's parser cannot
decide the statement type of the line, it creates a command statement object,
keeping the original line. If it turns out this line was actually not a command
statement, the evaluator re-runs the parser.

### Line concatenation

In most programming languages, line concatenations by a backslash character or
comments are handled at a very early stage of language implementations. However,
GNU make changes the behavior for them depending on parse/eval context. For
example, the following Makefile outputs "has space" and "hasnospace":

    VAR := has\
    space
    all:
    	echo $(VAR)
    	echo has\
    nospace

GNU make usually inserts a whitespace between lines, but for commands it
doesn't. As we've seen in the previous subsection, we cannot decide a line is a
command statement or not. This means we should handle them after evaluating
statements. Similar discussion applies for comments.

We have some comment/backslash related testcases in the testcase directory of
kati's repository.

Parser for expressions
----------------------

A statement may have one or more expressions. The number of expressions a
statement has depends on its type. For example,

    A := $(X)

This is an assignment statement and has two expressions - *A* and *$(X)*. Types
of expressions and their parser are defined in *expr.cc*. Like other programming
languages, an expression is a tree of expressions. The type of a leaf expression
is literal, variable reference,
[substitution references](http://www.gnu.org/software/make/manual/make.html#Substitution-Refs),
and make functions.

As written, backslashes and comments change their behavior depending on the
context. Kati handles them in this phase. *ParseExprOpt* is the enum for the
contexts.

As a nature of old systems, GNU make is very permissive. For some reason, it
allows some kind of unmatched pairs of parentheses. For example, GNU make
doesn't think *$($(foo)* is an error - this is a reference to variable
*$(foo*. If you have some experiences with parsers, you may wonder how one can
implement a parser which allows such expressions. It seems GNU make
intentionally allows this:

http://git.savannah.gnu.org/cgit/make.git/tree/expand.c#n285

As GNU make allows this, some Makefiles have unmatched parentheses, so kati
shouldn't raise an error for them, unfortunately.

GNU make has a bunch of functions. Most users would use only simple ones such as
*$(wildcard ...)* and *$(subst ...)*. There are also more complex functions such
as *$(if ...)* and *$(call ...)*, which make GNU make Turing-complete. *func.cc*
defines them. Though *func.cc* is not short, they are fairly simple. As for
functions, I remember only one weirdness. Kati needs to slightly change its
parsing for *$(if ...)*, *$(and ...)*, and *$(or ...)*. See *trim_space* and
*trim_right_space_1st* in *func.h* and how they are used in *expr.cc*.

Evaluator for statements
------------------------

Evaluator for statements are defined in *eval.cc*. As written, there are four
kinds of statements:

* Rules
* Assignments
* Commands
* Make directives

There is nothing tricky around commands and make directives. A rule statement
have some forms and should be parsed after evaluating expression by the third
parser. This will be discussed in the next section.

Assignments in GNU make is tricky a bit. There are two kinds of variables in GNU
make - simple variables and recursive variables. See the following code snippet:

    A = $(info world!)   # recursive
    B := $(info Hello,)  # simple
    $(A)
    $(B)

This code outputs "Hello," and "world!", in this order. The evaluation of
a recursive variable is delayed until the variable is referenced. So the first
line, which is an assignment for a recursive variable, outputs nothing. The
content of the variable *$(A)* will be *$(info world!)*. The assignment in the
second line uses *:=* which means this is a simple variable. For simple
variables, the right hand side is evaluated immediately. So "Hello," will be
output and the value of *$(B)* will be an empty string ($(info ...) returns an
empty string). "world!" will be shown when the third line is evaluated, and
lastly the forth line does nothing, as *$(B)* is an empty string.

There are two more kinds of assignments - *+=* and *?=*. These assignments keep
the type of the original variable. Evaluation of them will be done immediately
only when the left hand side of the assignment is already defined and is a
simple variable.

The combination of *+=* and target specific variables make things more
complicated. Here is an example code:

    all: test1 test2 test3 test4
    
    A:=X
    B=X
    X:=foo
    
    test1: A+=$(X)
    test1:
    	@echo $(A)  # X bar
    
    test2: B+=$(X)
    test2:
    	@echo $(B)  # X bar
    
    test3: A:=
    test3: A+=$(X)
    test3:
    	@echo $(A)  # foo
    
    test4: B=
    test4: B+=$(X)
    test4:
    	@echo $(B)  # bar
    
    X:=bar

*$(A)* in *test3* is a simple variable. Though *$(A)* in the global scope is
simple, *$(A)* in *test1* is a recursive variable. This means types of global
variables don't affect types of target specific variables. However, The result
of *test1* ("X bar") shows the value of a target specific variable is
concatenated to the value of a global variable.

Parser for rules
----------------

After evaluating a rule statement, kati needs to parse the evaluated result. A
rule statement can actually be the following four things:

* A rule
* A [target specific variable](http://www.gnu.org/software/make/manual/make.html#Target_002dspecific)
* An empty line
* An error (there're non-whitespace characters without a colon)

Parsing them is mostly done in *rule.cc*.

### Rules

A rule is something like *all: hello.exe*. You should be familiar with it. There
are some kinds rule definitions such as pattern rules, double colon rules, and
order only dependencies, but they don't complicate the rule parser.

A tricky feature for the parser is semicolon. You can write a command in the
same line as the rule. For example,

    target:
    	echo hi!

and

    target: ; echo hi!

have the same meaning. This is tricky because kati shouldn't evaluate expressions
in a command until the command is actually invoked. As a semicolon can appear as
the result of expression evaluation, there are some corner cases. A tricky
example:

    all: $(info foo) ; $(info bar)
    $(info baz)

should output *foo*, *baz*, and then *bar*, in this order, but

    VAR := all: $(info foo) ; $(info bar)
    $(VAR)
    $(info baz)

outputs *foo*, *bar*, and then *baz*.

### Target specific variables

You may not familiar with target specific variables. This feature allows you to
define variable which can be referenced only from commands in a specified
target. See the following code:

    VAR := X
    target1: VAR := Y
    target1:
    	echo $(VAR)
    target2:
    	echo $(VAR)

In this example, *target1* shows *Y* and *target2* shows *X*. I think this
feature is somewhat similar to namespaces in other programming languages. If a
target specific variable is specified for a non-leaf target, the variable will
be used even in building dependent targets.

This is the only GNU make's feature I don't like. See the following Makefile:

    hello: CFLAGS := -g
    hello: hello.o
    	gcc $(CFLAGS) $< -o $@
    hello.o: hello.c
    	gcc $(CFLAGS) -c $< -o $@

If you run make for the target *hello*, *CFLAGS* is applied for both commands:

    $ make hello
    gcc -g -c hello.c -o hello.o
    gcc -g hello.o -o hello

However, *CFLAGS* for *hello* won't be used when you build *hello.o* only:

    $ make hello.o
    gcc  -c hello.c -o hello.o

Things could be even worse when two targets with different target specific
variables depend on a same target. The build result will be inconsistent. I
think there is no valid usage of this feature for non-leaf targets.

Let's go back to the parsing. Like for semicolons, we need to delay the
evaluation of the right hand side of the assignment for recursive variables. Its
implementation is very similar to the one for semicolons, but the combination of
the assignment and the semicolon makes parsing a bit trickier. An example:

    target1: ;X=Y echo $(X)  # A rule with a command
    target2: X=;Y echo $(X)  # A target specific variable

Evaluator for expressions
-------------------------

Evaluation of expressions is done in *expr.cc*, *func.cc*, and
*command.cc*. The amount of code for this step is fairly large especially
because of the number of GNU make functions. However, their implementations are
fairly straightforward.

One tricky function is $(wildcard ...). It seems GNU make is doing some kind of
optimization only for this function and $(wildcard ...) in commands seem to be
evaluated before the evaluation phase for commands. Both C++ kati and Go kati
are different from GNU make's behavior in different ways, but it seems this
incompatibility is OK for Android build.

There is an important optimization done for Android. Android's build system has
a lot of $(shell find ...) calls to create a list of all .java/.mk files under a
directory, and they are slow. For this, kati has a builtin emulator of GNU
find. The find emulator traverses the directory tree first and creates an
in-memory directory tree. Then the find emulator returns results of find
commands using the cached tree. For my environment, the find command emulator
makes kati ~1.6x faster.

The implementations of some IO-related functions in commands are tricky in the
ninja generation mode. This will be described later.

Dependency builder
------------------

Now we get a list of rules and a variable table. *dep.cc* builds a dependency
graph using the list of rules. I think this step is what GNU make is supposed to
do for normal users.

This step is also fairly complex but there's nothing strange. There are three
kinds of rules in GNU make:

* explicit rule
* implicit rule
* suffix rule

The following code shows the three kinds:

    all: foo.o
    foo.o:
    	echo explicit
    %.o:
    	echo implicit
    .c.o:
    	echo suffix

In the above example, all of these three rules match the target *foo.o*. GNU
make prioritizes explicit rules first. When there's no explicit rule for a
target, it uses an implicit rule with longer pattern string. Suffix rules are
used only when there are no explicit/implicit rules.

Android has more than one thousand implicit rules and there are ten thousands of
targets. It's too slow to do matching for them with a naive O(NM)
algorithm. Kati uses a trie to speed up this step.

Multiple rules without commands should be merged into the rule with a
command. For example:

    foo.o: foo.h
    %.o: %.c
    	$(CC) -c $< -o $@

*foo.o* depends not only on *foo.c*, but also on *foo.h*.

Executor
--------

C++ kati's executor is fairly simple. This is defined in *exec.cc*. This is
useful only for testing because this lacks some important features for a build
system (e.g., parallel build).

Expressions in commands are evaluated at this stage. When they are evaluated,
target specific variables and some special variables (e.g., $< and $@) should be
considered. *command.cc* is handling them and this file is used by both the
executor and the ninja generator.

Ninja generator
---------------

*ninja.cc* generates a ninja file using the results of other components. This
step is actually fairly complicated because kati needs to map GNU make's
features to ninja's.

A build rule in GNU make may have multiple commands, while ninja's has always a
single command. To mitigate this, the ninja generator translates multiple
commands into something like *(cmd1) && (cmd2) && ...*. Kati should also escape
some special characters for ninja and shell.

The tougher thing is $(shell ...) in commands. Current kati's implementation
translates it into shell's $(...). This works for many cases. But this approach
won't work when the result of $(shell ...) is passed to another make
function. For example

    all:
    	echo $(if $(shell echo),FAIL,PASS)

should output PASS, because the result of $(shell echo) is an empty string. GNU
make and kati's executor mode output PASS correctly. However, kati's ninja
generator emits a ninja file which shows FAIL.

I wrote a few experimental patches for this issue, but they didn't
work. The current kati's implementation has an Android specific workaround for
this. See *HasNoIoInShellScript* in *func.cc* for detail.

Ninja regeneration
------------------

C++ kati has --regen flag. If this flag is specified, kati checks if anything
in your environment was changed after the previous run. If kati thinks it doesn't
need to regenerate the ninja file, it finishes quickly. For Android, running
kati takes ~30 secs at the first run but the second run takes only ~1 sec.

Kati thinks it needs to regenerate the ninja file when one of the followings is
changed:

* The command line flags passed to kati
* A timestamp of a Makefile used to generate the previous ninja file
* An environment variable used while evaluating Makefiles
* A result of $(wildcard ...)
* A result of $(shell ...)

Quickly doing the last check is tricky. It takes ~18 secs to run all $(shell
...) in Android's build system due to the slowness of $(shell find ...). So, for
find commands executed by kati's find emulator, kati stores the timestamps of
traversed directories with the find command itself. For each find commands, kati
checks the timestamps of them. If they are not changed, kati doesn't need to
re-run the find command.

Kati doesn't run $(shell date ...) and $(shell echo ...) during this check. The
former always changes so there's no sense to re-run them. Android uses the
latter to create a file and the result of them are empty strings. We don't want
to update these files to get empty strings.

TODO
----

A big TODO is sub-makes invoked by $(MAKE). I wrote some experimental patches
but nothing is ready to be used as of writing.
