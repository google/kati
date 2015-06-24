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

#include "dep.h"

#include <algorithm>
#include <memory>
#include <unordered_map>
#include <unordered_set>

#include "eval.h"
#include "fileutil.h"
#include "log.h"
#include "rule.h"
#include "strutil.h"
#include "var.h"

static vector<DepNode*>* g_dep_node_pool;

static StringPiece ReplaceSuffix(StringPiece s, StringPiece newsuf) {
  string r;
  AppendString(StripExt(s), &r);
  r += '.';
  AppendString(newsuf, &r);
  return Intern(r);
}

DepNode::DepNode(StringPiece o, bool p)
    : output(o),
      has_rule(false),
      is_order_only(false),
      is_phony(p),
      rule_vars(NULL) {
  g_dep_node_pool->push_back(this);
}

class DepBuilder {
 public:
  DepBuilder(Evaluator* ev,
             const vector<shared_ptr<Rule>>& rules,
             const unordered_map<StringPiece, Vars*>& rule_vars)
      : ev_(ev),
        rule_vars_(rule_vars),
        first_rule_(NULL) {
    PopulateRules(rules);
  }

  ~DepBuilder() {
  }

  void Build(vector<StringPiece> targets,
             vector<DepNode*>* nodes) {
    if (targets.empty()) {
      if (!first_rule_) {
        ERROR("*** No targets.");
      }
      CHECK(!first_rule_->outputs.empty());
      targets.push_back(first_rule_->outputs[0]);
    }

    // TODO: LogStats?

    for (StringPiece target : targets) {
      cur_rule_vars_.reset(new Vars);
      ev_->set_current_scope(cur_rule_vars_.get());
      DepNode* n = BuildPlan(target, "");
      nodes->push_back(n);
      ev_->set_current_scope(NULL);
      cur_rule_vars_.reset(NULL);
    }
  }

 private:
  bool Exists(StringPiece target) {
    auto found = rules_.find(target);
    if (found != rules_.end())
      return true;
    if (phony_.count(target))
      return true;
    return ::Exists(target);
  }

  void PopulateRules(const vector<shared_ptr<Rule>>& rules) {
    for (shared_ptr<Rule> rule : rules) {
      if (rule->outputs.empty()) {
        PopulateImplicitRule(rule);
      } else {
        PopulateExplicitRule(rule);
      }
    }
    reverse(implicit_rules_.begin(), implicit_rules_.end());
    for (auto& p : suffix_rules_) {
      reverse(p.second.begin(), p.second.end());
    }
  }

  bool PopulateSuffixRule(shared_ptr<Rule> rule, StringPiece output) {
    if (output.empty() || output[0] != '.')
      return false;

    StringPiece rest = output.substr(1);
    size_t dot_index = rest.find('.');
    // If there is only a single dot or the third dot, this is not a
    // suffix rule.
    if (dot_index == string::npos ||
        rest.substr(dot_index+1).find('.') != string::npos) {
      return false;
    }

    StringPiece input_suffix = rest.substr(0, dot_index);
    StringPiece output_suffix = rest.substr(dot_index+1);
    shared_ptr<Rule> r = make_shared<Rule>(*rule);
    r->inputs.clear();
    r->inputs.push_back(input_suffix);
    r->is_suffix_rule = true;
    suffix_rules_[output_suffix].push_back(r);
    return true;
  }

  shared_ptr<Rule> MergeRules(const Rule& old_rule,
                              const Rule& rule,
                              StringPiece output,
                              bool is_suffix_rule) {
    if (old_rule.is_double_colon != rule.is_double_colon) {
      ERROR("%s:%d: *** target file `%s' has both : and :: entries.",
            LOCF(rule.loc), output.as_string().c_str());
    }
    if (!old_rule.cmds.empty() && !rule.cmds.empty() &&
        !is_suffix_rule && !rule.is_double_colon) {
      WARN("%s:%d: warning: overriding commands for target `%s'",
           LOCF(rule.cmd_loc()), output.as_string().c_str());
      WARN("%s:%d: warning: ignoring old commands for target `%s'",
           LOCF(old_rule.cmd_loc()), output.as_string().c_str());
    }

    shared_ptr<Rule> r = make_shared<Rule>(rule);
    if (rule.is_double_colon) {
      r->cmds.clear();
      for (Value* c : old_rule.cmds)
        r->cmds.push_back(c);
      for (Value* c : rule.cmds)
        r->cmds.push_back(c);
    } else if (!old_rule.cmds.empty() && rule.cmds.empty()) {
      r->cmds = old_rule.cmds;
    }
    for (StringPiece p : old_rule.output_patterns) {
      r->output_patterns.push_back(p);
    }
    return r;
  }

  void PopulateExplicitRule(shared_ptr<Rule> rule) {
    for (StringPiece output : rule->outputs) {
      const bool is_suffix_rule = PopulateSuffixRule(rule, output);
      auto p = rules_.insert(make_pair(output, rule));
      if (p.second) {
        if (!first_rule_ && output.get(0) != '.') {
          first_rule_ = rule;
        }
      } else {
        p.first->second =
            MergeRules(*p.first->second, *rule, output, is_suffix_rule);
      }
    }
  }

  void PopulateImplicitRule(shared_ptr<Rule> rule) {
    for (StringPiece output_pattern : rule->output_patterns) {
      shared_ptr<Rule> r = make_shared<Rule>(*rule);
      r->output_patterns.clear();
      r->output_patterns.push_back(output_pattern);
      implicit_rules_.push_back(r);
    }
  }

  shared_ptr<Rule> LookupRule(StringPiece o) {
    auto found = rules_.find(o);
    if (found != rules_.end())
      return found->second;
    return NULL;
  }

  Vars* LookupRuleVars(StringPiece o) {
    auto found = rule_vars_.find(o);
    if (found != rule_vars_.end())
      return found->second;
    return NULL;
  }

  bool CanPickImplicitRule(shared_ptr<Rule> rule, StringPiece output) {
    CHECK(rule->output_patterns.size() == 1);
    Pattern pat(rule->output_patterns[0]);
    if (!pat.Match(output)) {
      return false;
    }
    for (StringPiece input : rule->inputs) {
      string buf;
      pat.AppendSubst(output, input, &buf);
      if (!Exists(buf))
        return false;
    }
    return true;
  }

  Vars* MergeImplicitRuleVars(StringPiece output, Vars* vars) {
    auto found = rule_vars_.find(output);
    if (found == rule_vars_.end())
      return vars;
    if (vars == NULL)
      return found->second;
    // TODO: leak.
    Vars* r = new Vars(*found->second);
    for (auto p : *vars) {
      (*r)[p.first] = p.second;
    }
    return r;
  }

  bool PickRule(StringPiece output,
                shared_ptr<Rule>* out_rule, Vars** out_var) {
    shared_ptr<Rule> rule = LookupRule(output);
    Vars* vars = LookupRuleVars(output);
    *out_rule = rule;
    *out_var = vars;
    if (rule) {
      if (!rule->cmds.empty()) {
        return true;
      }
    }

    for (shared_ptr<Rule> irule : implicit_rules_) {
      if (!CanPickImplicitRule(irule, output))
        continue;
      if (rule) {
        shared_ptr<Rule> r = make_shared<Rule>(*rule);
        r->output_patterns = irule->output_patterns;
        for (StringPiece input : irule->inputs)
          r->inputs.push_back(input);
        r->cmds = irule->cmds;
        r->loc = irule->loc;
        r->cmd_lineno = irule->cmd_lineno;
        *out_rule = r;
        return true;
      }
      if (vars) {
        CHECK(irule->output_patterns.size() == 1);
        vars = MergeImplicitRuleVars(irule->output_patterns[0], vars);
        *out_var = vars;
      }
      *out_rule = irule;
      return true;
    }

    StringPiece output_suffix = GetExt(output);
    if (output_suffix.get(0) != '.')
      return rule.get();
    output_suffix = output_suffix.substr(1);

    SuffixRuleMap::const_iterator found = suffix_rules_.find(output_suffix);
    if (found == suffix_rules_.end())
      return rule.get();

    for (shared_ptr<Rule> irule : found->second) {
      CHECK(irule->inputs.size() == 1);
      StringPiece input = ReplaceSuffix(output, irule->inputs[0]);
      if (!Exists(input))
        continue;

      if (rule) {
        shared_ptr<Rule> r = make_shared<Rule>(*rule);
        r->inputs.insert(r->inputs.begin(), input);
        r->cmds = irule->cmds;
        r->loc = irule->loc;
        r->cmd_lineno = irule->cmd_lineno;
        *out_rule = r;
        return true;
      }
      if (vars) {
        CHECK(irule->outputs.size() == 1);
        vars = MergeImplicitRuleVars(irule->outputs[0], vars);
        *out_var = vars;
      }
      *out_rule = irule;
      return true;
    }

    return rule.get();
  }

  DepNode* BuildPlan(StringPiece output, StringPiece needed_by) {
    LOG("BuildPlan: %s for %s",
        output.as_string().c_str(),
        needed_by.as_string().c_str());

    auto found = done_.find(output);
    if (found != done_.end()) {
      return found->second;
    }

    DepNode* n = new DepNode(output, phony_.count(output));
    done_[output] = n;

    shared_ptr<Rule> rule;
    Vars* vars;
    if (!PickRule(output, &rule, &vars)) {
      return n;
    }

    vector<unique_ptr<ScopedVar>> sv;
    if (vars) {
      for (const auto& p : *vars) {
        StringPiece name = p.first;
        RuleVar* var = reinterpret_cast<RuleVar*>(p.second);
        CHECK(var);
        Var* new_var = var->v();
        if (var->op() == AssignOp::PLUS_EQ) {
          Var* old_var = ev_->LookupVar(name);
          if (old_var->IsDefined()) {
            // TODO: This would be incorrect and has a leak.
            shared_ptr<string> s = make_shared<string>();
            old_var->Eval(ev_, s.get());
            if (!s->empty())
              *s += ' ';
            new_var->Eval(ev_, s.get());
            new_var = new SimpleVar(s, old_var->Origin());
          }
        } else if (var->op() == AssignOp::QUESTION_EQ) {
          Var* old_var = ev_->LookupVar(name);
          if (old_var->IsDefined()) {
            continue;
          }
        }
        sv.push_back(move(unique_ptr<ScopedVar>(
            new ScopedVar(cur_rule_vars_.get(), name, new_var))));
      }
    }

    for (StringPiece input : rule->inputs) {
      if (rule->output_patterns.size() > 0) {
        if (rule->output_patterns.size() > 1) {
          ERROR("TODO: multiple output pattern is not supported yet");
        }
        string o;
        Pattern(rule->output_patterns[0]).AppendSubst(output, input, &o);
        input = Intern(o);
      } else if (rule->is_suffix_rule) {
        input = Intern(ReplaceSuffix(output, input));
      }

      n->actual_inputs.push_back(input);
      DepNode* c = BuildPlan(input, output);
      n->deps.push_back(c);
    }

    for (StringPiece input : rule->order_only_inputs) {
      DepNode* c = BuildPlan(input, output);
      c->is_order_only = true;
      n->deps.push_back(c);
    }

    n->has_rule = true;
    n->cmds = rule->cmds;
    if (cur_rule_vars_->empty()) {
      n->rule_vars = NULL;
    } else {
      n->rule_vars = new Vars;
      for (auto p : *cur_rule_vars_) {
        n->rule_vars->insert(p);
      }
    }
    n->loc = rule->loc;
    if (!rule->cmds.empty() && rule->cmd_lineno)
      n->loc.lineno = rule->cmd_lineno;

    return n;
  }

  Evaluator* ev_;
  unordered_map<StringPiece, shared_ptr<Rule>> rules_;
  const unordered_map<StringPiece, Vars*>& rule_vars_;
  unique_ptr<Vars> cur_rule_vars_;

  vector<shared_ptr<Rule>> implicit_rules_;   // pattern=%. no prefix,suffix.
  //vector<Rule*> iprefix_rules_;   // pattern=prefix%..  may have suffix
  //vector<Rule*> isuffix_rules_;   // pattern=%suffix  no prefix
  typedef unordered_map<StringPiece, vector<shared_ptr<Rule>>> SuffixRuleMap;
  SuffixRuleMap suffix_rules_;

  shared_ptr<Rule> first_rule_;
  unordered_map<StringPiece, DepNode*> done_;
  unordered_set<StringPiece> phony_;
};

void MakeDep(Evaluator* ev,
             const vector<shared_ptr<Rule>>& rules,
             const unordered_map<StringPiece, Vars*>& rule_vars,
             const vector<StringPiece>& targets,
             vector<DepNode*>* nodes) {
  DepBuilder db(ev, rules, rule_vars);
  db.Build(targets, nodes);
}

void InitDepNodePool() {
  g_dep_node_pool = new vector<DepNode*>;
}

void QuitDepNodePool() {
  for (DepNode* n : *g_dep_node_pool)
    delete n;
  delete g_dep_node_pool;
}
