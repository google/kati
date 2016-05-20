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
#include <stdio.h>
#include <string.h>
#include <stdlib.h>
#include <time.h>
#include <unistd.h>

#include "affinity.h"
#include "dep.h"
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
#include "string_piece.h"
#include "stringprintf.h"
#include "strutil.h"
#include "symtab.h"
#include "timeutil.h"
#include "var.h"

static void Init() {
  InitSymtab();
  InitFuncTable();
  InitDepNodePool();
  InitParser();
}

static void Quit() {
  ReportAllStats();

  QuitParser();
  QuitDepNodePool();
  QuitFuncTable();
  QuitSymtab();
}

static void ReadBootstrapMakefile(const vector<Symbol>& targets,
                                  vector<Stmt*>* stmts) {
  string bootstrap = (
      "CC?=cc\n"
#if defined(__APPLE__)
      "CXX?=c++\n"
#else
      "CXX?=g++\n"
#endif
      "AR?=ar\n"
      // Pretend to be GNU make 3.81, for compatibility.
      "MAKE_VERSION?=3.81\n"
      "KATI?=ckati\n"
      // Overwrite $SHELL environment variable.
      "SHELL=/bin/sh\n"
      // TODO: Add more builtin vars.

      // http://www.gnu.org/software/make/manual/make.html#Catalogue-of-Rules
      // The document above is actually not correct. See default.c:
      // http://git.savannah.gnu.org/cgit/make.git/tree/default.c?id=4.1
      ".c.o:\n"
      "\t$(CC) $(CFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n"
      ".cc.o:\n"
      "\t$(CXX) $(CXXFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n"
      // TODO: Add more builtin rules.
                      );
  if (g_flags.generate_ninja) {
    bootstrap += StringPrintf("MAKE?=make -j%d\n",
                              g_flags.num_jobs <= 1 ? 1 : g_flags.num_jobs / 2);
  } else {
    bootstrap += StringPrintf("MAKE?=%s\n",
                              JoinStrings(g_flags.subkati_args, " ").c_str());
  }
  bootstrap += StringPrintf("MAKECMDGOALS?=%s\n",
                            JoinSymbols(targets, " ").c_str());

  char cwd[PATH_MAX];
  if (!getcwd(cwd, PATH_MAX)) {
    fprintf(stderr, "getcwd failed\n");
    CHECK(false);
  }
  bootstrap += StringPrintf("CURDIR:=%s\n", cwd);
  Parse(Intern(bootstrap).str(), Loc("*bootstrap*", 0), stmts);
}

static void SetVar(StringPiece l, VarOrigin origin) {
  size_t found = l.find('=');
  CHECK(found != string::npos);
  Symbol lhs = Intern(l.substr(0, found));
  StringPiece rhs = l.substr(found + 1);
  lhs.SetGlobalVar(
      new RecursiveVar(NewLiteral(rhs.data()), origin, rhs.data()));
}

extern "C" char** environ;

static int Run(const vector<Symbol>& targets,
               const vector<StringPiece>& cl_vars,
               const string& orig_args) {
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

  MakefileCacheManager* cache_mgr = NewMakefileCacheManager();

  Intern("MAKEFILE_LIST").SetGlobalVar(
      new SimpleVar(StringPrintf(" %s", g_flags.makefile), VarOrigin::FILE));
  for (char** p = environ; *p; p++) {
    SetVar(*p, VarOrigin::ENVIRONMENT);
  }
  Evaluator* ev = new Evaluator();

  vector<Stmt*> bootstrap_asts;
  ReadBootstrapMakefile(targets, &bootstrap_asts);
  ev->set_is_bootstrap(true);
  for (Stmt* stmt : bootstrap_asts) {
    LOG("%s", stmt->DebugString().c_str());
    stmt->Eval(ev);
  }
  ev->set_is_bootstrap(false);

  ev->set_is_commandline(true);
  for (StringPiece l : cl_vars) {
    vector<Stmt*> asts;
    Parse(Intern(l).str(), Loc("*bootstrap*", 0), &asts);
    CHECK(asts.size() == 1);
    asts[0]->Eval(ev);
  }
  ev->set_is_commandline(false);

  {
    ScopedTimeReporter tr("eval time");
    Makefile* mk = cache_mgr->ReadMakefile(g_flags.makefile);
    for (Stmt* stmt : mk->stmts()) {
      LOG("%s", stmt->DebugString().c_str());
      stmt->Eval(ev);
    }
  }

  for (ParseErrorStmt* err : GetParseErrors()) {
    WARN("%s:%d: warning for parse error in an unevaluated line: %s",
         LOCF(err->loc()), err->msg.c_str());
  }

  vector<DepNode*> nodes;
  {
    ScopedTimeReporter tr("make dep time");
    MakeDep(ev, ev->rules(), ev->rule_vars(), targets, &nodes);
  }

  if (g_flags.is_syntax_check_only)
    return 0;

  if (g_flags.generate_ninja) {
    ScopedTimeReporter tr("generate ninja time");
    GenerateNinja(nodes, ev, orig_args, start_time);
    return 0;
  }

  for (const auto& p : ev->exports()) {
    const Symbol name = p.first;
    if (p.second) {
      Var* v = ev->LookupVar(name);
      const string&& value = v->Eval(ev);
      LOG("setenv(%s, %s)", name.c_str(), value.c_str());
      setenv(name.c_str(), value.c_str(), 1);
    } else {
      LOG("unsetenv(%s)", name.c_str());
      unsetenv(name.c_str());
    }
  }

  {
    ScopedTimeReporter tr("exec time");
    Exec(nodes, ev);
  }

  for (Stmt* stmt : bootstrap_asts)
    delete stmt;
  delete ev;
  delete cache_mgr;

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
  Init();
  string orig_args;
  for (int i = 0; i < argc; i++) {
    if (i)
      orig_args += ' ';
    orig_args += argv[i];
  }
  g_flags.Parse(argc, argv);
  FindFirstMakefie();
  if (g_flags.makefile == NULL)
    ERROR("*** No targets specified and no makefile found.");
  // This depends on command line flags.
  if (g_flags.use_find_emulator)
    InitFindEmulator();
  int r = Run(g_flags.targets, g_flags.cl_vars, orig_args);
  Quit();
  return r;
}
