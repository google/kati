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

#include <limits.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include <string_view>

#include "affinity.h"
#include "eval.h"
#include "exec.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "find.h"
#include "flags.h"
#include "func.h"
#include "log.h"
#include "ninja.h"
#include "parser.h"
#include "regen.h"
#include "stats.h"
#include "stmt.h"
#include "stringprintf.h"
#include "strutil.h"
#include "symtab.h"
#include "timeutil.h"
#include "var.h"

// We know that there are leaks in Kati. Turn off LeakSanitizer by default.
extern "C" const char* __asan_default_options() {
  return "detect_leaks=0:allow_user_segv_handler=1";
}

static void ReadBootstrapMakefile(const std::vector<Symbol>& targets,
                                  std::vector<Stmt*>* stmts) {
  std::string bootstrap =
      ("CC?=cc\n"
#if defined(__APPLE__)
       "CXX?=c++\n"
#else
       "CXX?=g++\n"
#endif
       "AR?=ar\n"
       // Pretend to be GNU make 4.2.1, for compatibility.
       "MAKE_VERSION?=4.2.1\n"
       "KATI?=ckati\n"
       // Overwrite $SHELL environment variable.
       "SHELL=/bin/sh\n"
       // TODO: Add more builtin vars.
      );

  if (!g_flags.no_builtin_rules) {
    bootstrap += (
        // http://www.gnu.org/software/make/manual/make.html#Catalogue-of-Rules
        // The document above is actually not correct. See default.c:
        // http://git.savannah.gnu.org/cgit/make.git/tree/default.c?id=4.1
        ".c.o:\n"
        "\t$(CC) $(CFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n"
        ".cc.o:\n"
        "\t$(CXX) $(CXXFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n"
        // TODO: Add more builtin rules.
    );
  }
  if (g_flags.generate_ninja) {
    bootstrap += StringPrintf("MAKE?=make -j%d\n",
                              g_flags.num_jobs <= 1 ? 1 : g_flags.num_jobs / 2);
  } else {
    bootstrap += StringPrintf("MAKE?=%s\n",
                              JoinStrings(g_flags.subkati_args, " ").c_str());
  }
  bootstrap +=
      StringPrintf("MAKECMDGOALS?=%s\n", JoinSymbols(targets, " ").c_str());

  char cwd[PATH_MAX];
  if (!getcwd(cwd, PATH_MAX)) {
    fprintf(stderr, "getcwd failed\n");
    CHECK(false);
  }
  bootstrap += StringPrintf("CURDIR:=%s\n", cwd);
  Parse(Intern(bootstrap).str(), Loc("*bootstrap*", 0), stmts);
}

static void SetVar(std::string_view l,
                   VarOrigin origin,
                   Frame* definition,
                   Loc loc) {
  size_t found = l.find('=');
  CHECK(found != std::string::npos);
  Symbol lhs = Intern(l.substr(0, found));
  std::string_view rhs = l.substr(found + 1);
  lhs.SetGlobalVar(new RecursiveVar(Value::NewLiteral(rhs.data()), origin,
                                    definition, loc, rhs.data()));
}

extern "C" char** environ;

class SegfaultHandler {
 public:
  explicit SegfaultHandler(Evaluator* ev);
  ~SegfaultHandler();

  void handle(int, siginfo_t*, void*);

 private:
  static SegfaultHandler* global_handler;

  void dumpstr(const char* s) const {
    (void)write(STDERR_FILENO, s, strlen(s));
  }
  void dumpint(int i) const {
    char buf[11];
    char* ptr = buf + sizeof(buf) - 1;

    if (i < 0) {
      i = -i;
      dumpstr("-");
    } else if (i == 0) {
      dumpstr("0");
      return;
    }

    *ptr = '\0';
    while (ptr > buf && i > 0) {
      *--ptr = '0' + (i % 10);
      i = i / 10;
    }

    dumpstr(ptr);
  }

  Evaluator* ev_;

  struct sigaction orig_action_;
  struct sigaction new_action_;
};

SegfaultHandler* SegfaultHandler::global_handler = nullptr;

SegfaultHandler::SegfaultHandler(Evaluator* ev) : ev_(ev) {
  CHECK(global_handler == nullptr);
  global_handler = this;

  // Construct an alternate stack, so that we can handle stack overflows.
  stack_t ss;
  ss.ss_sp = malloc(SIGSTKSZ * 2);
  CHECK(ss.ss_sp != nullptr);
  ss.ss_size = SIGSTKSZ * 2;
  ss.ss_flags = 0;
  if (sigaltstack(&ss, nullptr) == -1) {
    PERROR("sigaltstack");
  }

  // Register our segfault handler using the alternate stack, falling
  // back to the default handler.
  sigemptyset(&new_action_.sa_mask);
  new_action_.sa_flags = SA_ONSTACK | SA_SIGINFO | SA_RESETHAND;
  new_action_.sa_sigaction = [](int sig, siginfo_t* info, void* context) {
    if (global_handler != nullptr) {
      global_handler->handle(sig, info, context);
    }

    raise(SIGSEGV);
  };
  sigaction(SIGSEGV, &new_action_, &orig_action_);
}

void SegfaultHandler::handle(int sig, siginfo_t* info, void* context) {
  // Avoid fprintf in case it allocates or tries to do anything else that may
  // hang.
  dumpstr("*kati*: Segmentation fault, last evaluated line was ");
  dumpstr(ev_->loc().filename);
  dumpstr(":");
  dumpint(ev_->loc().lineno);
  dumpstr("\n");

  // Run the original handler, in case we've been preloaded with libSegFault
  // or similar.
  if (orig_action_.sa_sigaction != nullptr) {
    orig_action_.sa_sigaction(sig, info, context);
  }
}

SegfaultHandler::~SegfaultHandler() {
  sigaction(SIGSEGV, &orig_action_, nullptr);
  global_handler = nullptr;
}

static int Run(const std::vector<Symbol>& targets,
               const std::vector<std::string_view>& cl_vars,
               const std::string& orig_args) {
  double start_time = GetTime();

  if (g_flags.generate_ninja && (g_flags.regen || g_flags.dump_kati_stamp)) {
    ScopedTimeReporter tr("regen check time");
    if (!NeedsRegen(start_time, orig_args)) {
      fprintf(stderr, "No need to regenerate ninja file\n");
      return 0;
    }
    if (g_flags.dump_kati_stamp) {
      printf("Need to regenerate ninja file\n");
      return 0;
    }
    ClearGlobCache();
  }

  SetAffinityForSingleThread();

  Evaluator ev;
  if (!ev.Start()) {
    return 1;
  }
  Intern("MAKEFILE_LIST")
      .SetGlobalVar(new SimpleVar(StringPrintf(" %s", g_flags.makefile),
                                  VarOrigin::FILE, ev.CurrentFrame(),
                                  ev.loc()));
  for (char** p = environ; *p; p++) {
    SetVar(*p, VarOrigin::ENVIRONMENT, nullptr, Loc());
  }
  SegfaultHandler segfault(&ev);

  std::vector<Stmt*> bootstrap_asts;
  ReadBootstrapMakefile(targets, &bootstrap_asts);

  {
    ScopedFrame frame(ev.Enter(FrameType::PHASE, "*bootstrap*", Loc()));
    ev.in_bootstrap();
    for (Stmt* stmt : bootstrap_asts) {
      LOG("%s", stmt->DebugString().c_str());
      stmt->Eval(&ev);
    }
  }

  {
    ScopedFrame frame(ev.Enter(FrameType::PHASE, "*command line*", Loc()));
    ev.in_command_line();
    for (std::string_view l : cl_vars) {
      std::vector<Stmt*> asts;
      Parse(Intern(l).str(), Loc("*bootstrap*", 0), &asts);
      CHECK(asts.size() == 1);
      asts[0]->Eval(&ev);
    }
  }
  ev.in_toplevel_makefile();

  {
    ScopedFrame eval_frame(ev.Enter(FrameType::PHASE, "*parse*", Loc()));
    ScopedTimeReporter tr("eval time");

    ScopedFrame file_frame(ev.Enter(FrameType::PARSE, g_flags.makefile, Loc()));
    const Makefile& mk =
        MakefileCacheManager::Get().ReadMakefile(g_flags.makefile);
    for (Stmt* stmt : mk.stmts()) {
      LOG("%s", stmt->DebugString().c_str());
      stmt->Eval(&ev);
    }
  }

  for (ParseErrorStmt* err : GetParseErrors()) {
    WARN_LOC(err->loc(), "warning for parse error in an unevaluated line: %s",
             err->msg.c_str());
  }

  if (g_flags.dump_include_graph != nullptr) {
    ev.DumpIncludeJSON(std::string(g_flags.dump_include_graph));
  }

  std::vector<NamedDepNode> nodes;
  {
    ScopedFrame frame(
        ev.Enter(FrameType::PHASE, "*dependency analysis*", Loc()));
    ScopedTimeReporter tr("make dep time");
    MakeDep(&ev, ev.rules(), ev.rule_vars(), targets, &nodes);
  }

  if (g_flags.is_syntax_check_only)
    return 0;

  if (g_flags.generate_ninja) {
    ScopedFrame frame(ev.Enter(FrameType::PHASE, "*ninja generation*", Loc()));
    ScopedTimeReporter tr("generate ninja time");
    GenerateNinja(nodes, &ev, orig_args, start_time);
    ev.DumpStackStats();
    ev.Finish();
    return 0;
  }

  for (const auto& p : ev.exports()) {
    const Symbol name = p.first;
    if (p.second) {
      Var* v = ev.LookupVar(name);
      const std::string&& value = v->Eval(&ev);
      LOG("setenv(%s, %s)", name.c_str(), value.c_str());
      setenv(name.c_str(), value.c_str(), 1);
    } else {
      LOG("unsetenv(%s)", name.c_str());
      unsetenv(name.c_str());
    }
  }

  {
    ScopedFrame frame(ev.Enter(FrameType::PHASE, "*execution*", Loc()));
    ScopedTimeReporter tr("exec time");
    Exec(nodes, &ev);
  }

  ev.DumpStackStats();
  ev.Finish();

  for (Stmt* stmt : bootstrap_asts)
    delete stmt;

  return 0;
}

static void FindFirstMakefie() {
  if (g_flags.makefile != NULL)
    return;
  if (Exists("GNUmakefile")) {
    g_flags.makefile = "GNUmakefile";
#if !defined(__APPLE__)
  } else if (Exists("makefile")) {
    g_flags.makefile = "makefile";
#endif
  } else if (Exists("Makefile")) {
    g_flags.makefile = "Makefile";
  }
}

static void HandleRealpath(int argc, char** argv) {
  char buf[PATH_MAX];
  for (int i = 0; i < argc; i++) {
    if (realpath(argv[i], buf))
      printf("%s\n", buf);
  }
}

int main(int argc, char* argv[]) {
  if (argc >= 2 && !strcmp(argv[1], "--realpath")) {
    HandleRealpath(argc - 2, argv + 2);
    return 0;
  }
  std::string orig_args;
  for (int i = 0; i < argc; i++) {
    if (i)
      orig_args += ' ';
    orig_args += argv[i];
  }
  g_flags.Parse(argc, argv);
  if (g_flags.working_dir) {
    int ret = chdir(g_flags.working_dir);
    if (ret != 0)
      ERROR("*** %s: %s", g_flags.working_dir, strerror(errno));
  }
  FindFirstMakefie();
  if (g_flags.makefile == NULL)
    ERROR("*** No targets specified and no makefile found.");
  // This depends on command line flags.
  if (g_flags.use_find_emulator)
    InitFindEmulator();
  int r = Run(g_flags.targets, g_flags.cl_vars, orig_args);
  ReportAllStats();
  return r;
}
