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

#include "command.h"

#include <unordered_map>
#include <unordered_set>

#include "dep.h"
#include "eval.h"
#include "flags.h"
#include "log.h"
#include "strutil.h"
#include "var.h"

namespace {

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
  AutoVar(CommandEvaluator* ce, const char* sym) : ce_(ce), sym_(sym) {}
  virtual ~AutoVar() = default;

  CommandEvaluator* ce_;
  const char* sym_;
};

#define DECLARE_AUTO_VAR_CLASS(name)                                    \
  class name : public AutoVar {                                         \
   public:                                                              \
   name(CommandEvaluator* ce, const char* sym)                          \
       : AutoVar(ce, sym) {}                                            \
   virtual ~name() = default;                                           \
   virtual void Eval(Evaluator* ev, string* s) const override;          \
  }

DECLARE_AUTO_VAR_CLASS(AutoAtVar);
DECLARE_AUTO_VAR_CLASS(AutoLessVar);
DECLARE_AUTO_VAR_CLASS(AutoHatVar);
DECLARE_AUTO_VAR_CLASS(AutoPlusVar);
DECLARE_AUTO_VAR_CLASS(AutoStarVar);
DECLARE_AUTO_VAR_CLASS(AutoNotImplementedVar);

class AutoSuffixDVar : public AutoVar {
 public:
  AutoSuffixDVar(CommandEvaluator* ce, const char* sym, Var* wrapped)
      : AutoVar(ce, sym), wrapped_(wrapped) {
  }
  virtual ~AutoSuffixDVar() = default;
  virtual void Eval(Evaluator* ev, string* s) const override;

 private:
  Var* wrapped_;
};

class AutoSuffixFVar : public AutoVar {
 public:
  AutoSuffixFVar(CommandEvaluator* ce, const char* sym, Var* wrapped)
      : AutoVar(ce, sym), wrapped_(wrapped) {}
  virtual ~AutoSuffixFVar() = default;
  virtual void Eval(Evaluator* ev, string* s) const override;

 private:
  Var* wrapped_;
};

void AutoAtVar::Eval(Evaluator*, string* s) const {
  *s += ce_->current_dep_node()->output.str();
}

void AutoLessVar::Eval(Evaluator*, string* s) const {
  auto& ai = ce_->current_dep_node()->actual_inputs;
  if (!ai.empty())
    *s += ai[0].str();
}

void AutoHatVar::Eval(Evaluator*, string* s) const {
  unordered_set<StringPiece> seen;
  WordWriter ww(s);
  for (Symbol ai : ce_->current_dep_node()->actual_inputs) {
    if (seen.insert(ai.str()).second)
      ww.Write(ai.str());
  }
}

void AutoPlusVar::Eval(Evaluator*, string* s) const {
  WordWriter ww(s);
  for (Symbol ai : ce_->current_dep_node()->actual_inputs) {
    ww.Write(ai.str());
  }
}

void AutoStarVar::Eval(Evaluator*, string* s) const {
  const DepNode* n = ce_->current_dep_node();
  if (!n->output_pattern.IsValid())
    return;
  Pattern pat(n->output_pattern.str());
  pat.Stem(n->output.str()).AppendToString(s);
}

void AutoNotImplementedVar::Eval(Evaluator* ev, string*) const {
  ev->Error(StringPrintf(
      "Automatic variable `$%s' isn't supported yet", sym_));
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

void ParseCommandPrefixes(StringPiece* s, bool* echo, bool* ignore_error) {
  *s = TrimLeftSpace(*s);
  while (true) {
    char c = s->get(0);
    if (c == '@') {
      *echo = false;
    } else if (c == '-') {
      *ignore_error = true;
    } else if (c == '+') {
      // ignore recursion marker
    } else {
      break;
    }
    *s = TrimLeftSpace(s->substr(1));
  }
}

}  // namespace

CommandEvaluator::CommandEvaluator(Evaluator* ev)
    : ev_(ev) {
#define INSERT_AUTO_VAR(name, sym) do {                                 \
    Var* v = new name(this, sym);                                       \
    Intern(sym).SetGlobalVar(v);                                        \
    Intern(sym"D").SetGlobalVar(new AutoSuffixDVar(this, sym"D", v));   \
    Intern(sym"F").SetGlobalVar(new AutoSuffixFVar(this, sym"F", v));   \
  } while (0)
  INSERT_AUTO_VAR(AutoAtVar, "@");
  INSERT_AUTO_VAR(AutoLessVar, "<");
  INSERT_AUTO_VAR(AutoHatVar, "^");
  INSERT_AUTO_VAR(AutoPlusVar, "+");
  INSERT_AUTO_VAR(AutoStarVar, "*");
  // TODO: Implement them.
  INSERT_AUTO_VAR(AutoNotImplementedVar, "%");
  INSERT_AUTO_VAR(AutoNotImplementedVar, "?");
  INSERT_AUTO_VAR(AutoNotImplementedVar, "|");
}

void CommandEvaluator::Eval(DepNode* n, vector<Command*>* commands) {
  ev_->set_loc(n->loc);
  ev_->set_current_scope(n->rule_vars);
  current_dep_node_ = n;
  for (Value* v : n->cmds) {
    const string&& cmds_buf = v->Eval(ev_);
    StringPiece cmds = cmds_buf;
    bool global_echo = !g_flags.is_silent_mode;
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
        Command* command = new Command(n->output);
        command->cmd = cmd.as_string();
        command->echo = echo;
        command->ignore_error = ignore_error;
        commands->push_back(command);
      }
      if (index == string::npos)
        break;
    }
    continue;
  }

  if (!ev_->delayed_output_commands().empty()) {
    vector<Command*> output_commands;
    for (const string& cmd : ev_->delayed_output_commands()) {
      Command* c = new Command(n->output);
      c->cmd = cmd;
      c->echo = false;
      c->ignore_error = false;
      output_commands.push_back(c);
    }
    // Prepend |output_commands|.
    commands->swap(output_commands);
    copy(output_commands.begin(), output_commands.end(),
         back_inserter(*commands));
    ev_->clear_delayed_output_commands();
  }

  ev_->set_current_scope(NULL);
}
