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

#include "ast.h"
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
#include "stats.h"
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

  if (g_flags.makefile == NULL) {
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
}

static void Quit() {
  ReportAllStats();

  QuitParser();
  QuitDepNodePool();
  QuitFuncTable();
  QuitSymtab();
}

static void ReadBootstrapMakefile(const vector<Symbol>& targets,
                                  vector<AST*>* asts) {
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
                              g_flags.num_jobs < 1 ? 1 : g_flags.num_jobs / 2);
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
  Parse(Intern(bootstrap).str(), Loc("*bootstrap*", 0), asts);
}

static void SetVar(StringPiece l, VarOrigin origin, Vars* vars) {
  size_t found = l.find('=');
  CHECK(found != string::npos);
  Symbol lhs = Intern(l.substr(0, found));
  StringPiece rhs = l.substr(found + 1);
  vars->Assign(lhs,
               new RecursiveVar(NewLiteral(rhs.data()), origin, rhs.data()));
}

extern "C" char** environ;

static int Run(const vector<Symbol>& targets,
               const vector<StringPiece>& cl_vars,
               const string& orig_args) {
  double start_time = GetTime();

  if (g_flags.generate_ninja && (g_flags.regen || g_flags.dump_kati_stamp)) {
    ScopedTimeReporter tr("regen check time");
    if (!NeedsRegen(g_flags.ninja_suffix, g_flags.ninja_dir,
                    g_flags.regen_ignoring_kati_binary,
                    g_flags.dump_kati_stamp,
                    start_time, orig_args)) {
      printf("No need to regenerate ninja file\n");
      return 0;
    }
    if (g_flags.dump_kati_stamp) {
      printf("Need to regenerate ninja file\n");
      return 0;
    }
    ClearGlobCache();
  }

  MakefileCacheManager* cache_mgr = NewMakefileCacheManager();

  Vars* vars = new Vars();
  for (char** p = environ; *p; p++) {
    SetVar(*p, VarOrigin::ENVIRONMENT, vars);
  }
  Evaluator* ev = new Evaluator(vars);

  vector<AST*> bootstrap_asts;
  ReadBootstrapMakefile(targets, &bootstrap_asts);
  ev->set_is_bootstrap(true);
  for (AST* ast : bootstrap_asts) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
  }
  ev->set_is_bootstrap(false);

  for (StringPiece l : cl_vars) {
    SetVar(l, VarOrigin::COMMAND_LINE, ev->mutable_vars());
  }

  vars->Assign(Intern("MAKEFILE_LIST"),
               new SimpleVar(StringPrintf(" %s", g_flags.makefile),
                             VarOrigin::FILE));

  {
    ScopedTimeReporter tr("eval time");
    Makefile* mk = cache_mgr->ReadMakefile(g_flags.makefile);
    for (AST* ast : mk->asts()) {
      LOG("%s", ast->DebugString().c_str());
      ast->Eval(ev);
    }
  }

  for (ParseErrorAST* err : GetParseErrors()) {
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
    GenerateNinja(g_flags.ninja_suffix, g_flags.ninja_dir,
                  nodes, ev, !targets.empty(),
                  orig_args, start_time);
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

  for (AST* ast : bootstrap_asts)
    delete ast;
  delete ev;
  delete vars;
  delete cache_mgr;

  return 0;
}

int main(int argc, char* argv[]) {
  Init();
  string orig_args;
  for (int i = 0; i < argc; i++) {
    if (i)
      orig_args += ' ';
    orig_args += argv[i];
  }
  g_flags.Parse(argc, argv);
  if (g_flags.makefile == NULL)
    ERROR("*** No targets specified and no makefile found.");
  // This depends on command line flags.
  if (g_flags.use_find_emulator)
    InitFindEmulator();
  int r = Run(g_flags.targets, g_flags.cl_vars, orig_args);
  Quit();
  return r;
}
