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
#include "flags.h"
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

class RuleTrie {
  struct Entry {
    Entry(const Rule* r, StringPiece s) : rule(r), suffix(s) {}
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

bool IsSuffixRule(Symbol output) {
  if (output.empty() || !IsSpecialTarget(output))
    return false;
  const StringPiece rest = StringPiece(output.str()).substr(1);
  size_t dot_index = rest.find('.');
  // If there is only a single dot or the third dot, this is not a
  // suffix rule.
  if (dot_index == string::npos ||
      rest.substr(dot_index + 1).find('.') != string::npos) {
    return false;
  }
  return true;
}

struct RuleMerger {
  vector<const Rule*> rules;
  vector<pair<Symbol, RuleMerger*>> implicit_outputs;
  vector<Symbol> validations;
  const Rule* primary_rule;
  const RuleMerger* parent;
  Symbol parent_sym;
  bool is_double_colon;

  RuleMerger()
      : primary_rule(nullptr), parent(nullptr), is_double_colon(false) {}

  void AddImplicitOutput(Symbol output, RuleMerger* merger) {
    implicit_outputs.push_back(make_pair(output, merger));
  }

  void AddValidation(Symbol validation) { validations.push_back(validation); }

  void SetImplicitOutput(Symbol output, Symbol p, const RuleMerger* merger) {
    if (!merger->primary_rule) {
      ERROR("*** implicit output `%s' on phony target `%s'", output.c_str(),
            p.c_str());
    }
    if (parent) {
      ERROR_LOC(merger->primary_rule->cmd_loc(),
                "*** implicit output `%s' of `%s' was already defined by `%s' "
                "at %s:%d",
                output.c_str(), p.c_str(), parent_sym.c_str(),
                LOCF(parent->primary_rule->cmd_loc()));
    }
    if (primary_rule) {
      ERROR_LOC(primary_rule->cmd_loc(),
                "*** implicit output `%s' may not have commands",
                output.c_str());
    }
    parent = merger;
    parent_sym = p;
  }

  void AddRule(Symbol output, const Rule* r) {
    if (rules.empty()) {
      is_double_colon = r->is_double_colon;
    } else if (is_double_colon != r->is_double_colon) {
      ERROR_LOC(r->loc, "*** target file `%s' has both : and :: entries.",
                output.c_str());
    }

    if (primary_rule && !r->cmds.empty() && !IsSuffixRule(output) &&
        !r->is_double_colon) {
      if (g_flags.werror_overriding_commands) {
        ERROR_LOC(r->cmd_loc(),
                  "*** overriding commands for target `%s', previously defined "
                  "at %s:%d",
                  output.c_str(), LOCF(primary_rule->cmd_loc()));
      } else {
        WARN_LOC(r->cmd_loc(), "warning: overriding commands for target `%s'",
                 output.c_str());
        WARN_LOC(primary_rule->cmd_loc(),
                 "warning: ignoring old commands for target `%s'",
                 output.c_str());
      }
      primary_rule = r;
    }
    if (!primary_rule && !r->cmds.empty()) {
      primary_rule = r;
    }

    rules.push_back(r);
  }

  void FillDepNodeFromRule(Symbol output, const Rule* r, DepNode* n) const {
    if (is_double_colon)
      copy(r->cmds.begin(), r->cmds.end(), back_inserter(n->cmds));

    ApplyOutputPattern(*r, output, r->inputs, &n->actual_inputs);
    ApplyOutputPattern(*r, output, r->order_only_inputs,
                       &n->actual_order_only_inputs);

    if (r->output_patterns.size() >= 1) {
      CHECK(r->output_patterns.size() == 1);
      n->output_pattern = r->output_patterns[0];
    }
  }

  void FillDepNodeLoc(const Rule* r, DepNode* n) const {
    n->loc = r->loc;
    if (!r->cmds.empty() && r->cmd_lineno)
      n->loc.lineno = r->cmd_lineno;
  }

  void FillDepNode(Symbol output, const Rule* pattern_rule, DepNode* n) const {
    if (primary_rule) {
      CHECK(!pattern_rule);
      FillDepNodeFromRule(output, primary_rule, n);
      FillDepNodeLoc(primary_rule, n);
      n->cmds = primary_rule->cmds;
    } else if (pattern_rule) {
      FillDepNodeFromRule(output, pattern_rule, n);
      FillDepNodeLoc(pattern_rule, n);
      n->cmds = pattern_rule->cmds;
    }

    for (const Rule* r : rules) {
      if (r == primary_rule)
        continue;
      FillDepNodeFromRule(output, r, n);
      if (n->loc.filename == NULL)
        n->loc = r->loc;
    }

    for (auto& implicit_output : implicit_outputs) {
      n->implicit_outputs.push_back(implicit_output.first);
      for (const Rule* r : implicit_output.second->rules) {
        FillDepNodeFromRule(output, r, n);
      }
    }

    for (auto& validation : validations) {
      n->actual_validations.push_back(validation);
    }
  }
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
      ninja_pool_var(NULL) {
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
        depfile_var_name_(Intern(".KATI_DEPFILE")),
        implicit_outputs_var_name_(Intern(".KATI_IMPLICIT_OUTPUTS")),
        ninja_pool_var_name_(Intern(".KATI_NINJA_POOL")),
        validations_var_name_(Intern(".KATI_VALIDATIONS")) {
    ScopedTimeReporter tr("make dep (populate)");
    PopulateRules(rules);
    // TODO?
    // LOG_STAT("%zu variables", ev->mutable_vars()->size());
    LOG_STAT("%zu explicit rules", rules_.size());
    LOG_STAT("%zu implicit rules", implicit_rules_->size());
    LOG_STAT("%zu suffix rules", suffix_rules_.size());

    HandleSpecialTargets();
  }

  void HandleSpecialTargets() {
    Loc loc;
    vector<Symbol> targets;

    if (GetRuleInputs(Intern(".PHONY"), &targets, &loc)) {
      for (Symbol t : targets)
        phony_.insert(t);
    }
    if (GetRuleInputs(Intern(".KATI_RESTAT"), &targets, &loc)) {
      for (Symbol t : targets)
        restat_.insert(t);
    }
    if (GetRuleInputs(Intern(".SUFFIXES"), &targets, &loc)) {
      if (targets.empty()) {
        suffix_rules_.clear();
      } else {
        WARN_LOC(loc, "kati doesn't support .SUFFIXES with prerequisites");
      }
    }

    // Note we can safely ignore .DELETE_ON_ERROR for --ninja mode.
    static const char* kUnsupportedBuiltinTargets[] = {".DEFAULT",
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
                                                       NULL};
    for (const char** p = kUnsupportedBuiltinTargets; *p; p++) {
      if (GetRuleInputs(Intern(*p), &targets, &loc)) {
        WARN_LOC(loc, "kati doesn't support %s", *p);
      }
    }
  }

  ~DepBuilder() {}

  void Build(vector<Symbol> targets, vector<NamedDepNode>* nodes) {
    if (!first_rule_.IsValid()) {
      ERROR("*** No targets.");
    }

    if (!g_flags.gen_all_targets && targets.empty()) {
      targets.push_back(first_rule_);
    }
    if (g_flags.gen_all_targets) {
      SymbolSet non_root_targets;
      for (const auto& p : rules_) {
        if (IsSpecialTarget(p.first))
          continue;
        for (const Rule* r : p.second.rules) {
          for (Symbol t : r->inputs)
            non_root_targets.insert(t);
          for (Symbol t : r->order_only_inputs)
            non_root_targets.insert(t);
        }
      }

      for (const auto& p : rules_) {
        Symbol t = p.first;
        if (!non_root_targets.exists(t) && !IsSpecialTarget(t)) {
          targets.push_back(p.first);
        }
      }
    }

    // TODO: LogStats?

    for (Symbol target : targets) {
      cur_rule_vars_.reset(new Vars);
      ev_->set_current_scope(cur_rule_vars_.get());
      DepNode* n = BuildPlan(target, Intern(""));
      nodes->push_back({target, n});
      ev_->set_current_scope(NULL);
      cur_rule_vars_.reset(NULL);
    }
  }

 private:
  bool Exists(Symbol target) {
    return (rules_.find(target) != rules_.end()) || phony_.exists(target) ||
           ::Exists(target.str());
  }

  bool GetRuleInputs(Symbol s, vector<Symbol>* o, Loc* l) {
    auto found = rules_.find(s);
    if (found == rules_.end())
      return false;

    o->clear();
    CHECK(!found->second.rules.empty());
    *l = found->second.rules.front()->loc;
    for (const Rule* r : found->second.rules) {
      for (Symbol i : r->inputs)
        o->push_back(i);
    }
    return true;
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
    for (auto& p : rules_) {
      auto vars = LookupRuleVars(p.first);
      if (!vars) {
        continue;
      }
      auto var = vars->Lookup(implicit_outputs_var_name_);
      if (var->IsDefined()) {
        string implicit_outputs;
        var->Eval(ev_, &implicit_outputs);

        for (StringPiece output : WordScanner(implicit_outputs)) {
          Symbol sym = Intern(TrimLeadingCurdir(output));
          rules_[sym].SetImplicitOutput(sym, p.first, &p.second);
          p.second.AddImplicitOutput(sym, &rules_[sym]);
        }
      }

      var = vars->Lookup(validations_var_name_);
      if (var->IsDefined()) {
        string validations;
        var->Eval(ev_, &validations);

        for (StringPiece validation : WordScanner(validations)) {
          Symbol sym = Intern(TrimLeadingCurdir(validation));
          p.second.AddValidation(sym);
        }
      }
    }
  }

  bool PopulateSuffixRule(const Rule* rule, Symbol output) {
    if (!IsSuffixRule(output))
      return false;

    if (g_flags.werror_suffix_rules) {
      ERROR_LOC(rule->loc, "*** suffix rules are obsolete: %s", output.c_str());
    } else if (g_flags.warn_suffix_rules) {
      WARN_LOC(rule->loc, "warning: suffix rules are deprecated: %s",
               output.c_str());
    }

    const StringPiece rest = StringPiece(output.str()).substr(1);
    size_t dot_index = rest.find('.');

    StringPiece input_suffix = rest.substr(0, dot_index);
    StringPiece output_suffix = rest.substr(dot_index + 1);
    shared_ptr<Rule> r = make_shared<Rule>(*rule);
    r->inputs.clear();
    r->inputs.push_back(Intern(input_suffix));
    r->is_suffix_rule = true;
    suffix_rules_[output_suffix].push_back(r);
    return true;
  }

  void PopulateExplicitRule(const Rule* rule) {
    for (Symbol output : rule->outputs) {
      if (!first_rule_.IsValid() && !IsSpecialTarget(output)) {
        first_rule_ = output;
      }
      rules_[output].AddRule(output, rule);
      PopulateSuffixRule(rule, output);
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
    return (i == "RCS/%,v" || i == "RCS/%" || i == "%,v" || i == "s.%" ||
            i == "SCCS/s.%");
  }

  void PopulateImplicitRule(const Rule* rule) {
    for (Symbol output_pattern : rule->output_patterns) {
      if (output_pattern.str() != "%" || !IsIgnorableImplicitRule(rule)) {
        if (g_flags.werror_implicit_rules) {
          ERROR_LOC(rule->loc, "*** implicit rules are obsolete: %s",
                    output_pattern.c_str());
        } else if (g_flags.warn_implicit_rules) {
          WARN_LOC(rule->loc, "warning: implicit rules are deprecated: %s",
                   output_pattern.c_str());
        }

        implicit_rules_->Add(output_pattern.str(), rule);
      }
    }
  }

  const RuleMerger* LookupRuleMerger(Symbol o) {
    auto found = rules_.find(o);
    if (found != rules_.end()) {
      return &found->second;
    }
    return nullptr;
  }

  Vars* LookupRuleVars(Symbol o) {
    auto found = rule_vars_.find(o);
    if (found != rule_vars_.end())
      return found->second;
    return nullptr;
  }

  bool CanPickImplicitRule(const Rule* rule,
                           Symbol output,
                           DepNode* n,
                           shared_ptr<Rule>* out_rule) {
    Symbol matched;
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

  bool PickRule(Symbol output,
                DepNode* n,
                const RuleMerger** out_rule_merger,
                shared_ptr<Rule>* pattern_rule,
                Vars** out_var) {
    const RuleMerger* rule_merger = LookupRuleMerger(output);
    Vars* vars = LookupRuleVars(output);
    *out_rule_merger = rule_merger;
    *out_var = vars;
    if (rule_merger && rule_merger->primary_rule) {
      for (auto implicit_output : rule_merger->implicit_outputs) {
        vars = MergeImplicitRuleVars(implicit_output.first, vars);
      }
      *out_var = vars;
      return true;
    }

    vector<const Rule*> irules;
    implicit_rules_->Get(output.str(), &irules);
    for (auto iter = irules.rbegin(); iter != irules.rend(); ++iter) {
      if (!CanPickImplicitRule(*iter, output, n, pattern_rule))
        continue;
      if (rule_merger) {
        return true;
      }
      CHECK((*pattern_rule)->output_patterns.size() == 1);
      vars = MergeImplicitRuleVars((*pattern_rule)->output_patterns[0], vars);
      *out_var = vars;
      return true;
    }

    StringPiece output_suffix = GetExt(output.str());
    if (output_suffix.get(0) != '.')
      return rule_merger != nullptr;
    output_suffix = output_suffix.substr(1);

    SuffixRuleMap::const_iterator found = suffix_rules_.find(output_suffix);
    if (found == suffix_rules_.end())
      return rule_merger != nullptr;

    for (const shared_ptr<Rule>& irule : found->second) {
      CHECK(irule->inputs.size() == 1);
      Symbol input = ReplaceSuffix(output, irule->inputs[0]);
      if (!Exists(input))
        continue;

      *pattern_rule = irule;
      if (rule_merger != nullptr)
        return true;
      if (vars) {
        CHECK(irule->outputs.size() == 1);
        vars = MergeImplicitRuleVars(irule->outputs[0], vars);
        *out_var = vars;
      }
      return true;
    }

    return rule_merger != nullptr;
  }

  DepNode* BuildPlan(Symbol output, Symbol needed_by UNUSED) {
    LOG("BuildPlan: %s for %s", output.c_str(), needed_by.c_str());

    auto found = done_.find(output);
    if (found != done_.end()) {
      return found->second;
    }

    DepNode* n =
        new DepNode(output, phony_.exists(output), restat_.exists(output));
    done_[output] = n;

    const RuleMerger* rule_merger = nullptr;
    shared_ptr<Rule> pattern_rule;
    Vars* vars;
    if (!PickRule(output, n, &rule_merger, &pattern_rule, &vars)) {
      return n;
    }
    if (rule_merger && rule_merger->parent) {
      output = rule_merger->parent_sym;
      done_[output] = n;
      n->output = output;
      if (!PickRule(output, n, &rule_merger, &pattern_rule, &vars)) {
        return n;
      }
    }

    if (rule_merger)
      rule_merger->FillDepNode(output, pattern_rule.get(), n);
    else
      RuleMerger().FillDepNode(output, pattern_rule.get(), n);

    vector<unique_ptr<ScopedVar>> sv;
    ScopedFrame frame(ev_->Enter(FrameType::DEPENDENCY, output.str(), n->loc));

    if (vars) {
      for (const auto& p : *vars) {
        Symbol name = p.first;
        Var* var = p.second;
        CHECK(var);
        Var* new_var = var;
        if (var->op() == AssignOp::PLUS_EQ) {
          Var* old_var = ev_->LookupVar(name);
          if (old_var->IsDefined()) {
            // TODO: This would be incorrect and has a leak.
            shared_ptr<string> s = make_shared<string>();
            old_var->Eval(ev_, s.get());
            if (!s->empty())
              *s += ' ';
            new_var->Eval(ev_, s.get());
            new_var =
                new SimpleVar(*s, old_var->Origin(), frame.Current(), n->loc);
          }
        } else if (var->op() == AssignOp::QUESTION_EQ) {
          Var* old_var = ev_->LookupVar(name);
          if (old_var->IsDefined()) {
            continue;
          }
        }

        if (name == depfile_var_name_) {
          n->depfile_var = new_var;
        } else if (name == implicit_outputs_var_name_) {
        } else if (name == validations_var_name_) {
        } else if (name == ninja_pool_var_name_) {
          n->ninja_pool_var = new_var;
        } else {
          sv.emplace_back(new ScopedVar(cur_rule_vars_.get(), name, new_var));
        }
      }
    }

    if (g_flags.warn_phony_looks_real && n->is_phony &&
        output.str().find("/") != string::npos) {
      if (g_flags.werror_phony_looks_real) {
        ERROR_LOC(
            n->loc,
            "*** PHONY target \"%s\" looks like a real file (contains a \"/\")",
            output.c_str());
      } else {
        WARN_LOC(n->loc,
                 "warning: PHONY target \"%s\" looks like a real file "
                 "(contains a \"/\")",
                 output.c_str());
      }
    }

    if (!g_flags.writable.empty() && !n->is_phony) {
      bool found = false;
      for (const auto& w : g_flags.writable) {
        if (StringPiece(output.str()).starts_with(w)) {
          found = true;
          break;
        }
      }
      if (!found) {
        if (g_flags.werror_writable) {
          ERROR_LOC(n->loc, "*** writing to readonly directory: \"%s\"",
                    output.c_str());
        } else {
          WARN_LOC(n->loc, "warning: writing to readonly directory: \"%s\"",
                   output.c_str());
        }
      }
    }

    for (Symbol output : n->implicit_outputs) {
      done_[output] = n;

      if (g_flags.warn_phony_looks_real && n->is_phony &&
          output.str().find("/") != string::npos) {
        if (g_flags.werror_phony_looks_real) {
          ERROR_LOC(n->loc,
                    "*** PHONY target \"%s\" looks like a real file (contains "
                    "a \"/\")",
                    output.c_str());
        } else {
          WARN_LOC(n->loc,
                   "warning: PHONY target \"%s\" looks like a real file "
                   "(contains a \"/\")",
                   output.c_str());
        }
      }

      if (!g_flags.writable.empty() && !n->is_phony) {
        bool found = false;
        for (const auto& w : g_flags.writable) {
          if (StringPiece(output.str()).starts_with(w)) {
            found = true;
            break;
          }
        }
        if (!found) {
          if (g_flags.werror_writable) {
            ERROR_LOC(n->loc, "*** writing to readonly directory: \"%s\"",
                      output.c_str());
          } else {
            WARN_LOC(n->loc, "warning: writing to readonly directory: \"%s\"",
                     output.c_str());
          }
        }
      }
    }

    for (Symbol input : n->actual_inputs) {
      DepNode* c = BuildPlan(input, output);
      n->deps.push_back({input, c});

      bool is_phony = c->is_phony;
      if (!is_phony && !c->has_rule && g_flags.top_level_phony) {
        is_phony = input.str().find("/") == string::npos;
      }
      if (!n->is_phony && is_phony) {
        if (g_flags.werror_real_to_phony) {
          ERROR_LOC(n->loc,
                    "*** real file \"%s\" depends on PHONY target \"%s\"",
                    output.c_str(), input.c_str());
        } else if (g_flags.warn_real_to_phony) {
          WARN_LOC(n->loc,
                   "warning: real file \"%s\" depends on PHONY target \"%s\"",
                   output.c_str(), input.c_str());
        }
      }
    }

    for (Symbol input : n->actual_order_only_inputs) {
      DepNode* c = BuildPlan(input, output);
      n->order_onlys.push_back({input, c});
    }

    for (Symbol validation : n->actual_validations) {
      if (!g_flags.use_ninja_validations) {
        ERROR_LOC(
            n->loc,
            ".KATI_VALIDATIONS not allowed without --use_ninja_validations");
      }
      DepNode* c = BuildPlan(validation, output);
      n->validations.push_back({validation, c});
    }

    // Block on werror_writable/werror_phony_looks_real, because otherwise we
    // can't rely on is_phony being valid for this check.
    if (!n->is_phony && n->cmds.empty() && g_flags.werror_writable &&
        g_flags.werror_phony_looks_real) {
      if (n->deps.empty() && n->order_onlys.empty()) {
        if (g_flags.werror_real_no_cmds_or_deps) {
          ERROR_LOC(
              n->loc,
              "*** target \"%s\" has no commands or deps that could create it",
              output.c_str());
        } else if (g_flags.warn_real_no_cmds_or_deps) {
          WARN_LOC(n->loc,
                   "warning: target \"%s\" has no commands or deps that could "
                   "create it",
                   output.c_str());
        }
      } else {
        if (n->actual_inputs.size() == 1) {
          if (g_flags.werror_real_no_cmds) {
            ERROR_LOC(n->loc,
                      "*** target \"%s\" has no commands. Should \"%s\" be "
                      "using .KATI_IMPLICIT_OUTPUTS?",
                      output.c_str(), n->actual_inputs[0].c_str());
          } else if (g_flags.warn_real_no_cmds) {
            WARN_LOC(n->loc,
                     "warning: target \"%s\" has no commands. Should \"%s\" be "
                     "using .KATI_IMPLICIT_OUTPUTS?",
                     output.c_str(), n->actual_inputs[0].c_str());
          }
        } else {
          if (g_flags.werror_real_no_cmds) {
            ERROR_LOC(
                n->loc,
                "*** target \"%s\" has no commands that could create output "
                "file. Is a dependency missing .KATI_IMPLICIT_OUTPUTS?",
                output.c_str());
          } else if (g_flags.warn_real_no_cmds) {
            WARN_LOC(
                n->loc,
                "warning: target \"%s\" has no commands that could create "
                "output file. Is a dependency missing .KATI_IMPLICIT_OUTPUTS?",
                output.c_str());
          }
        }
      }
    }

    n->has_rule = true;
    n->is_default_target = first_rule_ == output;
    if (cur_rule_vars_->empty()) {
      n->rule_vars = NULL;
    } else {
      n->rule_vars = new Vars;
      for (auto p : *cur_rule_vars_) {
        n->rule_vars->insert(p);
      }
    }

    return n;
  }

  Evaluator* ev_;
  map<Symbol, RuleMerger> rules_;
  const unordered_map<Symbol, Vars*>& rule_vars_;
  unique_ptr<Vars> cur_rule_vars_;

  unique_ptr<RuleTrie> implicit_rules_;
  typedef unordered_map<StringPiece, vector<shared_ptr<Rule>>> SuffixRuleMap;
  SuffixRuleMap suffix_rules_;

  Symbol first_rule_;
  unordered_map<Symbol, DepNode*> done_;
  SymbolSet phony_;
  SymbolSet restat_;
  Symbol depfile_var_name_;
  Symbol implicit_outputs_var_name_;
  Symbol ninja_pool_var_name_;
  Symbol validations_var_name_;
};

void MakeDep(Evaluator* ev,
             const vector<const Rule*>& rules,
             const unordered_map<Symbol, Vars*>& rule_vars,
             const vector<Symbol>& targets,
             vector<NamedDepNode>* nodes) {
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

bool IsSpecialTarget(Symbol output) {
  return output.get(0) == '.' && output.get(1) != '.';
}
