#include <limits.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#include "ast.h"
#include "dep.h"
#include "eval.h"
#include "exec.h"
#include "file.h"
#include "file_cache.h"
#include "fileutil.h"
#include "func.h"
#include "log.h"
#include "parser.h"
#include "string_piece.h"
#include "stringprintf.h"
#include "strutil.h"
#include "var.h"

static const char* g_makefile;

static void ParseCommandLine(int argc, char* argv[],
                             vector<StringPiece>* targets) {
  for (int i = 1; i < argc; i++) {
    const char* arg = argv[i];
    if (!strcmp(arg, "-f")) {
      g_makefile = argv[++i];
    } else if (arg[0] == '-') {
      ERROR("Unknown flag: %s", arg);
    } else {
      targets->push_back(Intern(arg));
    }
  }
}

static void Init() {
  InitSymtab();
  InitFuncTable();
  InitDepNodePool();
  InitParser();

  if (g_makefile == NULL) {
    if (Exists("GNUmakefile")) {
      g_makefile = "GNUmakefile";
    } else if (Exists("makefile")) {
      g_makefile = "makefile";
    } else if (Exists("Makefile")) {
      g_makefile = "Makefile";
    } else {
      ERROR("*** No targets specified and no makefile found.");
    }
  }
}

static void Quit() {
  QuitParser();
  QuitDepNodePool();
  QuitFuncTable();
  QuitSymtab();
}

static void ReadBootstrapMakefile(const vector<StringPiece>& targets,
                                  vector<AST*>* asts) {
  string bootstrap = (
      "CC:=cc\n"
      "CXX:=g++\n"
      "AR:=ar\n"
      "MAKE:=kati\n"
      // Pretend to be GNU make 3.81, for compatibility.
      "MAKE_VERSION:=3.81\n"
      "SHELL:=/bin/sh\n"
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
  bootstrap += StringPrintf("MAKECMDGOALS:=%s\n",
                            JoinStrings(targets, " ").c_str());

  char cwd[PATH_MAX];
  if (!getcwd(cwd, PATH_MAX)) {
    fprintf(stderr, "getcwd failed\n");
    CHECK(false);
  }
  bootstrap += StringPrintf("CURDIR:=%s\n", cwd);
  Parse(Intern(bootstrap), Loc("*bootstrap*", 0), asts);
}

static int Run(const vector<StringPiece>& targets) {
  MakefileCacheManager* cache_mgr = NewMakefileCacheManager();

  // TODO: Fill env, etc.
  Vars* vars = new Vars();
  Evaluator* ev = new Evaluator(vars);

  vector<AST*> bootstrap_asts;
  ReadBootstrapMakefile(targets, &bootstrap_asts);
  ev->set_is_bootstrap(true);
  for (AST* ast : bootstrap_asts) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
  }
  ev->set_is_bootstrap(false);

  Makefile* mk = cache_mgr->ReadMakefile(g_makefile);
  for (AST* ast : mk->asts()) {
    LOG("%s", ast->DebugString().c_str());
    ast->Eval(ev);
  }

  EvalResult* er = ev->GetEvalResult();
  for (auto p : *er->vars) {
    vars->Assign(p.first, p.second);
  }
  er->vars->clear();

  vector<DepNode*> nodes;
  MakeDep(er->rules, *vars, er->rule_vars, targets, &nodes);

  Exec(nodes, vars);

  for (AST* ast : bootstrap_asts)
    delete ast;
  delete er;
  delete ev;
  delete vars;
  delete cache_mgr;

  return mk != 0;
}

int main(int argc, char* argv[]) {
  Init();
  vector<StringPiece> targets;
  ParseCommandLine(argc, argv, &targets);
  int r = Run(targets);
  Quit();
  return r;
}
