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
#include <iterator>
#include <map>
#include <memory>
#include <unordered_map>
#include <unordered_set>

#include "eval.h"
#include "fileutil.h"
#include "log.h"
#include "rule.h"
#include "stats.h"
#include "strutil.h"
#include "symtab.h"
#include "timeutil.h"
#include "var.h"

namespace {

static vector<DepNode*>* g_dep_node_pool;

static Symbol ReplaceSuffix(Symbol s, Symbol newsuf) {
  string r;
  AppendString(StripExt(s.str()), &r);
  r += '.';
  AppendString(newsuf.str(), &r);
  return Intern(r);
}

class RuleTrie {
  struct Entry {
    Entry(const Rule* r, StringPiece s)
        : rule(r), suffix(s) {
    }
    const Rule* rule;
    StringPiece suffix;
  };

 public:
  RuleTrie() {}
  ~RuleTrie() {
    for (auto& p : children_)
      delete p.second;
  }

  void Add(StringPiece name, const Rule* rule) {
    if (name.empty() || name[0] == '%') {
      rules_.push_back(Entry(rule, name));
      return;
    }
    const char c = name[0];
    auto p = children_.emplace(c, nullptr);
    if (p.second) {
      p.first->second = new RuleTrie();
    }
    p.first->second->Add(name.substr(1), rule);
  }

  void Get(StringPiece name, vector<const Rule*>* rules) const {
    for (const Entry& ent : rules_) {
      if ((ent.suffix.empty() && name.empty()) ||
          HasSuffix(name, ent.suffix.substr(1))) {
        rules->push_back(ent.rule);
      }
    }
    if (name.empty())
      return;
    auto found = children_.find(name[0]);
    if (found != children_.end()) {
      found->second->Get(name.substr(1), rules);
    }
  }

  size_t size() const {
    size_t r = rules_.size();
    for (const auto& c : children_)
      r += c.second->size();
    return r;
  }

 private:
  vector<Entry> rules_;
  unordered_map<char, RuleTrie*> children_;
};

}  // namespace

DepNode::DepNode(Symbol o, bool p, bool r)
    : output(o),
      has_rule(false),
      is_default_target(false),
      is_phony(p),
      is_restat(r),
      rule_vars(NULL),
      depfile_var(NULL),
      output_pattern(Symbol::IsUninitialized()) {
  g_dep_node_pool->push_back(this);
}

class DepBuilder {
 public:
  DepBuilder(Evaluator* ev,
             const vector<const Rule*>& rules,
             const unordered_map<Symbol, Vars*>& rule_vars)
      : ev_(ev),
        rule_vars_(rule_vars),
        implicit_rules_(new RuleTrie()),
        first_rule_(Symbol::IsUninitialized{}),
        depfile_var_name_(Intern(".KATI_DEPFILE")) {
    ScopedTimeReporter tr("make dep (populate)");
    PopulateRules(rules);
    LOG_STAT("%zu variables", ev->mutable_vars()->size());
    LOG_STAT("%zu explicit rules", rules_.size());
    LOG_STAT("%zu implicit rules", implicit_rules_->size());
    LOG_STAT("%zu suffix rules", suffix_rules_.size());

    auto found = rules_.find(Intern(".PHONY"));
    if (found != rules_.end()) {
      for (Symbol input : found->second->inputs) {
        phony_.insert(input);
      }
    }
    found = rules_.find(Intern(".KATI_RESTAT"));
    if (found != rules_.end()) {
      for (Symbol input : found->second->inputs) {
        restat_.insert(input);
      }
    }
    found = rules_.find(Intern(".SUFFIXES"));
    if (found != rules_.end()) {
      if (found->second->inputs.empty()) {
        suffix_rules_.clear();
      } else {
        WARN("%s:%d: kati doesn't support .SUFFIXES with prerequisites",
             LOCF(found->second->loc));
      }
    }

    // Note we can safely ignore .DELETE_ON_ERROR for --ninja mode.
    static const char* kUnsupportedBuiltinTargets[] = {
      ".DEFAULT",
      ".PRECIOUS",
      ".INTERMEDIATE",
      ".SECONDARY",
      ".SECONDEXPANSION",
      ".IGNORE",
      ".LOW_RESOLUTION_TIME",
      ".SILENT",
      ".EXPORT_ALL_VARIABLES",
      ".NOTPARALLEL",
      ".ONESHELL",
      ".POSIX",
      NULL
    };
    for (const char** p = kUnsupportedBuiltinTargets; *p; p++) {
      auto found = rules_.find(Intern(*p));
      if (found != rules_.end()) {
        WARN("%s:%d: kati doesn't support %s", LOCF(found->second->loc), *p);
      }
    }
  }

  ~DepBuilder() {
  }

  void Build(vector<Symbol> targets, vector<DepNode*>* nodes) {
    if (!first_rule_.IsValid()) {
      ERROR("*** No targets.");
    }

    if (!g_flags.gen_all_targets && targets.empty()) {
      targets.push_back(first_rule_);
    }
    if (g_flags.gen_all_targets) {
      unordered_set<Symbol> non_root_targets;
      for (const auto& p : rules_) {
        for (Symbol t : p.second->inputs)
          non_root_targets.insert(t);
        for (Symbol t : p.second->order_only_inputs)
          non_root_targets.insert(t);
      }

      for (const auto& p : rules_) {
        Symbol t = p.first;
        if (!non_root_targets.count(t)) {
          targets.push_back(p.first);
        }
      }
    }

    // TODO: LogStats?

    for (Symbol target : targets) {
      cur_rule_vars_.reset(new Vars);
      ev_->set_current_scope(cur_rule_vars_.get());
      DepNode* n = BuildPlan(target, Intern(""));
      nodes->push_back(n);
      ev_->set_current_scope(NULL);
      cur_rule_vars_.reset(NULL);
    }
  }

 private:
  bool Exists(Symbol target) {
    auto found = rules_.find(target);
    if (found != rules_.end())
      return true;
    if (phony_.count(target))
      return true;
    return ::Exists(target.str());
  }

  void PopulateRules(const vector<const Rule*>& rules) {
    for (const Rule* rule : rules) {
      if (rule->outputs.empty()) {
        PopulateImplicitRule(rule);
      } else {
        PopulateExplicitRule(rule);
      }
    }
    for (auto& p : suffix_rules_) {
      reverse(p.second.begin(), p.second.end());
    }
  }

  bool PopulateSuffixRule(const Rule* rule, Symbol output) {
    if (output.empty() || output.str()[0] != '.')
      return false;

    const StringPiece rest = StringPiece(output.str()).substr(1);
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
    r->inputs.push_back(Intern(input_suffix));
    r->is_suffix_rule = true;
    suffix_rules_[output_suffix].push_back(r);
    return true;
  }

  void ApplyOutputPattern(const Rule& r,
                          Symbol output,
                          const vector<Symbol>& inputs,
                          vector<Symbol>* out_inputs) {
    if (inputs.empty())
      return;
    if (r.is_suffix_rule) {
      for (Symbol input : inputs) {
        out_inputs->push_back(ReplaceSuffix(output, input));
      }
      return;
    }
    if (r.output_patterns.empty()) {
      copy(inputs.begin(), inputs.end(), back_inserter(*out_inputs));
      return;
    }
    CHECK(r.output_patterns.size() == 1);
    Pattern pat(r.output_patterns[0].str());
    for (Symbol input : inputs) {
      string buf;
      pat.AppendSubst(output.str(), input.str(), &buf);
      out_inputs->push_back(Intern(buf));
    }
  }

  shared_ptr<Rule> MergeRules(const Rule& old_rule,
                              const Rule& rule,
                              Symbol output,
                              bool is_suffix_rule) {
    COLLECT_STATS("make dep (merge rule)");
    if (old_rule.is_double_colon != rule.is_double_colon) {
      ERROR("%s:%d: *** target file `%s' has both : and :: entries.",
            LOCF(rule.loc), output.str().c_str());
    }
    if (!old_rule.cmds.empty() && !rule.cmds.empty() &&
        !is_suffix_rule && !rule.is_double_colon) {
      WARN("%s:%d: warning: overriding commands for target `%s'",
           LOCF(rule.cmd_loc()), output.str().c_str());
      WARN("%s:%d: warning: ignoring old commands for target `%s'",
           LOCF(old_rule.cmd_loc()), output.str().c_str());
    }

    shared_ptr<Rule> r = make_shared<Rule>(rule);
    if (rule.is_double_colon) {
      r->cmds.clear();
      for (Value* c : old_rule.cmds)
        r->cmds.push_back(c);
      for (Value* c : rule.cmds)
        r->cmds.push_back(c);
      if (!rule.output_patterns.empty() && !old_rule.output_patterns.empty() &&
          rule.output_patterns != old_rule.output_patterns) {
        ERROR("%s:%d: TODO: merging two double colon rules with output "
              "patterns is not supported", LOCF(rule.loc));
      }
    } else if (!old_rule.cmds.empty() && rule.cmds.empty()) {
      r->cmds = old_rule.cmds;
    }

    // If the latter rule has a command (regardless of the commands in
    // |old_rule|), inputs in the latter rule has a priority.
    if (rule.cmds.empty()) {
      r->inputs = old_rule.inputs;
      ApplyOutputPattern(rule, output, rule.inputs, &r->inputs);
      r->order_only_inputs = old_rule.order_only_inputs;
      ApplyOutputPattern(rule, output, rule.order_only_inputs,
                         &r->order_only_inputs);
      r->output_patterns = old_rule.output_patterns;
    } else {
      ApplyOutputPattern(old_rule, output, old_rule.inputs, &r->inputs);
      ApplyOutputPattern(old_rule, output, old_rule.order_only_inputs,
                         &r->order_only_inputs);
    }
    r->is_default_target |= old_rule.is_default_target;
    return r;
  }

  void PopulateExplicitRule(const Rule* orig_rule) {
    for (Symbol output : orig_rule->outputs) {
      const bool is_suffix_rule = PopulateSuffixRule(orig_rule, output);

      shared_ptr<Rule> rule = make_shared<Rule>(*orig_rule);
      rule->outputs.clear();
      rule->outputs.push_back(output);

      auto p = rules_.emplace(output, rule);
      if (p.second) {
        if (!first_rule_.IsValid() && output.get(0) != '.') {
          rule->is_default_target = true;
          first_rule_ = output;
        }
      } else {
        p.first->second =
            MergeRules(*p.first->second, *rule, output, is_suffix_rule);
      }
    }
  }

  static bool IsIgnorableImplicitRule(const Rule* rule) {
    // As kati doesn't have RCS/SCCS related default rules, we can
    // safely ignore suppression for them.
    if (rule->inputs.size() != 1)
      return false;
    if (!rule->order_only_inputs.empty())
      return false;
    if (!rule->cmds.empty())
      return false;
    const string& i = rule->inputs[0].str();
    return (i == "RCS/%,v" || i == "RCS/%" || i == "%,v" ||
            i == "s.%" || i == "SCCS/s.%");
  }

  void PopulateImplicitRule(const Rule* rule) {
    for (Symbol output_pattern : rule->output_patterns) {
      if (output_pattern.str() != "%" || !IsIgnorableImplicitRule(rule))
        implicit_rules_->Add(output_pattern.str(), rule);
    }
  }

  shared_ptr<Rule> LookupRule(Symbol o) {
    auto found = rules_.find(o);
    if (found != rules_.end())
      return found->second;
    return NULL;
  }

  Vars* LookupRuleVars(Symbol o) {
    auto found = rule_vars_.find(o);
    if (found != rule_vars_.end())
      return found->second;
    return NULL;
  }

  bool CanPickImplicitRule(const Rule* rule, Symbol output, DepNode* n,
                           shared_ptr<Rule>* out_rule) {
    Symbol matched(Symbol::IsUninitialized{});
    for (Symbol output_pattern : rule->output_patterns) {
      Pattern pat(output_pattern.str());
      if (pat.Match(output.str())) {
        bool ok = true;
        for (Symbol input : rule->inputs) {
          string buf;
          pat.AppendSubst(output.str(), input.str(), &buf);
          if (!Exists(Intern(buf))) {
            ok = false;
            break;
          }
        }

        if (ok) {
          matched = output_pattern;
          break;
        }
      }
    }
    if (!matched.IsValid())
      return false;

    *out_rule = make_shared<Rule>(*rule);
    if ((*out_rule)->output_patterns.size() > 1) {
      // We should mark all other output patterns as used.
      Pattern pat(matched.str());
      for (Symbol output_pattern : rule->output_patterns) {
        if (output_pattern == matched)
          continue;
        string buf;
        pat.AppendSubst(output.str(), output_pattern.str(), &buf);
        done_[Intern(buf)] = n;
      }
      (*out_rule)->output_patterns.clear();
      (*out_rule)->output_patterns.push_back(matched);
    }

    return true;
  }

  Vars* MergeImplicitRuleVars(Symbol output, Vars* vars) {
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

  bool PickRule(Symbol output, DepNode* n,
                shared_ptr<Rule>* out_rule, Vars** out_var) {
    COLLECT_STATS("make dep (pick rule)");
    shared_ptr<Rule> rule = LookupRule(output);
    Vars* vars = LookupRuleVars(output);
    *out_rule = rule;
    *out_var = vars;
    if (rule) {
      if (!rule->cmds.empty()) {
        return true;
      }
    }

    vector<const Rule*> irules;
    implicit_rules_->Get(output.str(), &irules);
    for (auto iter = irules.rbegin(); iter != irules.rend(); ++iter) {
      shared_ptr<Rule> irule;
      if (!CanPickImplicitRule(*iter, output, n, &irule))
        continue;
      if (rule) {
        shared_ptr<Rule> r = make_shared<Rule>(*rule);
        r->output_patterns = irule->output_patterns;
        r->inputs.clear();
        r->inputs = irule->inputs;
        copy(rule->inputs.begin(), rule->inputs.end(),
             back_inserter(r->inputs));
        r->cmds = irule->cmds;
        r->is_default_target |= irule->is_default_target;
        r->loc = irule->loc;
        r->cmd_lineno = irule->cmd_lineno;
        *out_rule = r;
        return true;
      }
      CHECK(irule->output_patterns.size() == 1);
      vars = MergeImplicitRuleVars(irule->output_patterns[0], vars);
      *out_var = vars;
      *out_rule = make_shared<Rule>(*irule);
      return true;
    }

    StringPiece output_suffix = GetExt(output.str());
    if (output_suffix.get(0) != '.')
      return rule.get();
    output_suffix = output_suffix.substr(1);

    SuffixRuleMap::const_iterator found = suffix_rules_.find(output_suffix);
    if (found == suffix_rules_.end())
      return rule.get();

    for (shared_ptr<Rule> irule : found->second) {
      CHECK(irule->inputs.size() == 1);
      Symbol input = ReplaceSuffix(output, irule->inputs[0]);
      if (!Exists(input))
        continue;

      if (rule) {
        shared_ptr<Rule> r = make_shared<Rule>(*rule);
        r->inputs.insert(r->inputs.begin(), input);
        r->cmds = irule->cmds;
        r->is_default_target |= irule->is_default_target;
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

  DepNode* BuildPlan(Symbol output, Symbol needed_by UNUSED) {
    LOG("BuildPlan: %s for %s",
        output.c_str(),
        needed_by.c_str());

    auto found = done_.find(output);
    if (found != done_.end()) {
      return found->second;
    }

    DepNode* n = new DepNode(output,
                             phony_.count(output),
                             restat_.count(output));
    done_[output] = n;

    shared_ptr<Rule> rule;
    Vars* vars;
    if (!PickRule(output, n, &rule, &vars)) {
      return n;
    }

    if (rule->output_patterns.size() >= 1) {
      CHECK(rule->output_patterns.size() == 1);
      n->output_pattern = rule->output_patterns[0];
    }

    vector<unique_ptr<ScopedVar>> sv;
    if (vars) {
      COLLECT_STATS("make dep (create scope)");
      for (const auto& p : *vars) {
        Symbol name = p.first;
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
            new_var = new SimpleVar(*s, old_var->Origin());
          }
        } else if (var->op() == AssignOp::QUESTION_EQ) {
          Var* old_var = ev_->LookupVar(name);
          if (old_var->IsDefined()) {
            continue;
          }
        }

        if (name == depfile_var_name_) {
          n->depfile_var = new_var;
        } else {
          sv.emplace_back(new ScopedVar(cur_rule_vars_.get(), name, new_var));
        }
      }
    }

    ApplyOutputPattern(*rule, output, rule->inputs, &n->actual_inputs);
    for (Symbol input : n->actual_inputs) {
      DepNode* c = BuildPlan(input, output);
      n->deps.push_back(c);
    }

    vector<Symbol> order_only_inputs;
    ApplyOutputPattern(*rule, output, rule->order_only_inputs,
                       &order_only_inputs);
    for (Symbol input : order_only_inputs) {
      DepNode* c = BuildPlan(input, output);
      n->order_onlys.push_back(c);
    }

    n->has_rule = true;
    n->cmds = rule->cmds;
    n->is_default_target = rule->is_default_target;
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
  map<Symbol, shared_ptr<Rule>> rules_;
  const unordered_map<Symbol, Vars*>& rule_vars_;
  unique_ptr<Vars> cur_rule_vars_;

  unique_ptr<RuleTrie> implicit_rules_;
  typedef unordered_map<StringPiece, vector<shared_ptr<Rule>>> SuffixRuleMap;
  SuffixRuleMap suffix_rules_;

  Symbol first_rule_;
  unordered_map<Symbol, DepNode*> done_;
  unordered_set<Symbol> phony_;
  unordered_set<Symbol> restat_;
  Symbol depfile_var_name_;
};

void MakeDep(Evaluator* ev,
             const vector<const Rule*>& rules,
             const unordered_map<Symbol, Vars*>& rule_vars,
             const vector<Symbol>& targets,
             vector<DepNode*>* nodes) {
  DepBuilder db(ev, rules, rule_vars);
  ScopedTimeReporter tr("make dep (build)");
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
