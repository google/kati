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
  virtual const char* Origin() const override {
    return "automatic";
  }

  virtual bool IsDefined() const override { CHECK(false); }
  virtual void AppendVar(Evaluator*, Value*) override { CHECK(false); }

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
  explicit Executor(Vars* vars)
      : vars_(vars),
        ev_(new Evaluator(vars_)) {
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

    LOG("ExecNode: %s for %s",
        n->output.as_string().c_str(),
        needed_by ? needed_by->output.as_string().c_str() : "(null)");

    for (DepNode* d : n->deps) {
#if 0
      if (d.is_order_only && exists(d->output)) {
      }
#endif
      ExecNode(d, n);
    }

    vector<Runner*> runners;
    CreateRunners(n, &runners);
    for (Runner* runner : runners) {
      if (runner->echo) {
        printf("%s\n", runner->cmd->c_str());
        fflush(stdout);
      }
      system(runner->cmd->c_str());
      delete runner;
    }
  }

  void CreateRunners(DepNode* n, vector<Runner*>* runners) {
    current_dep_node_ = n;
    for (Value* v : n->cmds) {
      shared_ptr<string> cmd = v->Eval(ev_.get());
      while (true) {
        size_t index = cmd->find('\n');
        if (index == string::npos)
          break;

        Runner* runner = new Runner;
        runner->output = n->output;
        runner->cmd = make_shared<string>(cmd->substr(0, index));
        runners->push_back(runner);
        cmd = make_shared<string>(cmd->substr(index + 1));
      }
      Runner* runner = new Runner;
      runner->output = n->output;
      runner->cmd = cmd;
      runners->push_back(runner);
      continue;
    }
  }

  const DepNode* current_dep_node() const { return current_dep_node_; }

 private:
  Vars* vars_;
  unique_ptr<Evaluator> ev_;
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

void Exec(const vector<DepNode*>& roots, Vars* vars) {
  unique_ptr<Executor> executor(new Executor(vars));
  for (DepNode* root : roots) {
    executor->ExecNode(root, NULL);
  }
}
