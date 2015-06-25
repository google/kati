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

#include "exec.h"

#include <stdio.h>
#include <stdlib.h>

#include <memory>
#include <unordered_map>
#include <unordered_set>
#include <utility>
#include <vector>

#include "dep.h"
#include "eval.h"
#include "fileutil.h"
#include "flags.h"
#include "log.h"
#include "string_piece.h"
#include "strutil.h"
#include "value.h"
#include "var.h"

namespace {

class Executor;

class AutoVar : public Var {
 public:
  virtual const char* Flavor() const override {
    return "undefined";
  }
  virtual VarOrigin Origin() const override {
    return VarOrigin::AUTOMATIC;
  }

  virtual void AppendVar(Evaluator*, Value*) override { CHECK(false); }

  virtual StringPiece String() const override {
    ERROR("$(value %s) is not implemented yet", sym_);
    return "";
  }

  virtual string DebugString() const override {
    return string("AutoVar(") + sym_ + ")";
  }

 protected:
  AutoVar(Executor* ex, const char* sym) : ex_(ex), sym_(sym) {}
  virtual ~AutoVar() = default;

  Executor* ex_;
  const char* sym_;
};

#define DECLARE_AUTO_VAR_CLASS(name)                            \
  class name : public AutoVar {                                 \
   public:                                                      \
   name(Executor* ex, const char* sym)                          \
       : AutoVar(ex, sym) {}                                    \
   virtual ~name() = default;                                   \
   virtual void Eval(Evaluator* ev, string* s) const override;  \
  }

DECLARE_AUTO_VAR_CLASS(AutoAtVar);
DECLARE_AUTO_VAR_CLASS(AutoLessVar);
DECLARE_AUTO_VAR_CLASS(AutoHatVar);
DECLARE_AUTO_VAR_CLASS(AutoPlusVar);
DECLARE_AUTO_VAR_CLASS(AutoStarVar);

class AutoSuffixDVar : public AutoVar {
 public:
  AutoSuffixDVar(Executor* ex, const char* sym, Var* wrapped)
      : AutoVar(ex, sym), wrapped_(wrapped) {
  }
  virtual ~AutoSuffixDVar() = default;
  virtual void Eval(Evaluator* ev, string* s) const override;

 private:
  Var* wrapped_;
};

class AutoSuffixFVar : public AutoVar {
 public:
  AutoSuffixFVar(Executor* ex, const char* sym, Var* wrapped)
      : AutoVar(ex, sym), wrapped_(wrapped) {}
  virtual ~AutoSuffixFVar() = default;
  virtual void Eval(Evaluator* ev, string* s) const override;

 private:
  Var* wrapped_;
};

struct Runner {
  Runner()
      : echo(true), ignore_error(false) {
  }
  StringPiece output;
  shared_ptr<string> cmd;
  bool echo;
  bool ignore_error;
  //StringPiece shell;
};

class Executor {
 public:
  explicit Executor(Evaluator* ev)
      : ev_(ev) {
    Vars* vars = ev_->mutable_vars();
#define INSERT_AUTO_VAR(name, sym) do {                                 \
      Var* v = new name(this, sym);                                     \
      (*vars)[STRING_PIECE(sym)] = v;                                   \
      (*vars)[STRING_PIECE(sym"D")] = new AutoSuffixDVar(this, sym"D", v); \
      (*vars)[STRING_PIECE(sym"F")] = new AutoSuffixFVar(this, sym"F", v); \
    } while (0)
    INSERT_AUTO_VAR(AutoAtVar, "@");
    INSERT_AUTO_VAR(AutoLessVar, "<");
    INSERT_AUTO_VAR(AutoHatVar, "^");
    INSERT_AUTO_VAR(AutoPlusVar, "+");
    INSERT_AUTO_VAR(AutoStarVar, "*");
  }

  void ExecNode(DepNode* n, DepNode* needed_by) {
    if (done_[n->output])
      return;
    done_[n->output] = true;

    LOG("ExecNode: %s for %s",
        n->output.as_string().c_str(),
        needed_by ? needed_by->output.as_string().c_str() : "(null)");

    for (DepNode* d : n->deps) {
      if (d->is_order_only && Exists(d->output)) {
        continue;
      }
      // TODO: Check the timestamp.
      if (Exists(d->output)) {
        continue;
      }
      ExecNode(d, n);
    }

    vector<Runner*> runners;
    CreateRunners(n, &runners);
    for (Runner* runner : runners) {
      if (runner->echo) {
        printf("%s\n", runner->cmd->c_str());
        fflush(stdout);
      }
      if (!g_is_syntax_check_only) {
        int result = system(runner->cmd->c_str());
        if (result != 0) {
          if (runner->ignore_error) {
            fprintf(stderr, "[%.*s] Error %d (ignored)\n",
                    SPF(runner->output), WEXITSTATUS(result));
          } else {
            fprintf(stderr, "*** [%.*s] Error %d\n",
                    SPF(runner->output), WEXITSTATUS(result));
            exit(1);
          }
        }
      }
      delete runner;
    }
  }

  void ParseCommandPrefixes(StringPiece* s, bool* echo, bool* ignore_error) {
    *s = TrimLeftSpace(*s);
    while (true) {
      char c = s->get(0);
      if (c == '@') {
        *echo = false;
      } else if (c == '-') {
        *ignore_error = true;
      } else {
        break;
      }
      *s = TrimLeftSpace(s->substr(1));
    }
  }

  void CreateRunners(DepNode* n, vector<Runner*>* runners) {
    ev_->set_current_scope(n->rule_vars);
    current_dep_node_ = n;
    for (Value* v : n->cmds) {
      shared_ptr<string> cmds_buf = v->Eval(ev_);
      StringPiece cmds = *cmds_buf;
      bool global_echo = true;
      bool global_ignore_error = false;
      ParseCommandPrefixes(&cmds, &global_echo, &global_ignore_error);
      if (cmds == "")
        continue;
      while (true) {
        size_t lf_cnt;
        size_t index = FindEndOfLine(cmds, 0, &lf_cnt);
        if (index == cmds.size())
          index = string::npos;
        StringPiece cmd = TrimLeftSpace(cmds.substr(0, index));
        cmds = cmds.substr(index + 1);

        bool echo = global_echo;
        bool ignore_error = global_ignore_error;
        ParseCommandPrefixes(&cmd, &echo, &ignore_error);

        if (!cmd.empty()) {
          Runner* runner = new Runner;
          runner->output = n->output;
          runner->cmd = make_shared<string>(cmd.as_string());
          runner->echo = echo;
          runner->ignore_error = ignore_error;
          runners->push_back(runner);
        }
        if (index == string::npos)
          break;
      }
      continue;
    }
    ev_->set_current_scope(NULL);
  }

  const DepNode* current_dep_node() const { return current_dep_node_; }

 private:
  Vars* vars_;
  Evaluator* ev_;
  unordered_map<StringPiece, bool> done_;
  DepNode* current_dep_node_;
};

void AutoAtVar::Eval(Evaluator*, string* s) const {
  AppendString(ex_->current_dep_node()->output, s);
}

void AutoLessVar::Eval(Evaluator*, string* s) const {
  auto& ai = ex_->current_dep_node()->actual_inputs;
  if (!ai.empty())
    AppendString(ai[0], s);
}

void AutoHatVar::Eval(Evaluator*, string* s) const {
  unordered_set<StringPiece> seen;
  WordWriter ww(s);
  for (StringPiece ai : ex_->current_dep_node()->actual_inputs) {
    if (seen.insert(ai).second)
      ww.Write(ai);
  }
}

void AutoPlusVar::Eval(Evaluator*, string* s) const {
  WordWriter ww(s);
  for (StringPiece ai : ex_->current_dep_node()->actual_inputs) {
    ww.Write(ai);
  }
}

void AutoStarVar::Eval(Evaluator*, string* s) const {
  AppendString(StripExt(ex_->current_dep_node()->output), s);
}

void AutoSuffixDVar::Eval(Evaluator* ev, string* s) const {
  string buf;
  wrapped_->Eval(ev, &buf);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(buf)) {
    ww.Write(Dirname(tok));
  }
}

void AutoSuffixFVar::Eval(Evaluator* ev, string* s) const {
  string buf;
  wrapped_->Eval(ev, &buf);
  WordWriter ww(s);
  for (StringPiece tok : WordScanner(buf)) {
    ww.Write(Basename(tok));
  }
}

}  // namespace

void Exec(const vector<DepNode*>& roots, Evaluator* ev) {
  unique_ptr<Executor> executor(new Executor(ev));
  for (DepNode* root : roots) {
    executor->ExecNode(root, NULL);
  }
}
