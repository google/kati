#include "dep.h"

#include <algorithm>
#include <memory>
#include <unordered_map>
#include <unordered_set>

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
      target_specific_vars(NULL) {
  g_dep_node_pool->push_back(this);
}

class DepBuilder {
 public:
  DepBuilder(const vector<shared_ptr<Rule>>& rules,
             const Vars& vars,
             const unordered_map<StringPiece, Vars*>& rule_vars)
      : vars_(vars),
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
      unique_ptr<Vars> tsvs(new Vars);
      DepNode* n = BuildPlan(target, "", tsvs.get());
      nodes->push_back(n);
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

  void PopulateExplicitRule(shared_ptr<Rule> rule) {
    for (StringPiece output : rule->outputs) {
      const bool is_suffix_rule = PopulateSuffixRule(rule, output);
      // isSuffixRule := db.populateSuffixRule(rule, output)


      /*
          if oldRule, present := db.rules[output]; present {
     r := mergeRules(oldRule, rule, output, isSuffixRule)
                                                         db.rules[output] = r
                                                         } else {
        db.rules[output] = rule
            if db.firstRule == nil && !strings.HasPrefix(output, ".") {
                db.firstRule = rule
              }
      }
      */

      auto p = rules_.insert(make_pair(output, rule));
      if (p.second) {
        if (!first_rule_ && output.get(0) != '.') {
          first_rule_ = rule;
        }
      } else {
        // TODO: merge
        CHECK(false);
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
        // TODO: Merge implicit variables...
        CHECK(false);
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
        // TODO: Merge implicit variables...
        CHECK(false);
      }
      *out_rule = irule;
      return true;
    }

    return rule.get();
  }

  DepNode* BuildPlan(StringPiece output, StringPiece needed_by, Vars* tsvs) {
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

    // TODO: Handle TSVs

    for (StringPiece input : rule->inputs) {
      if (rule->output_patterns.size() > 0) {
        if (rule->output_patterns.size() > 1) {
          ERROR("TODO: multiple output pattern is not supported yet");
        }
        string o;
        Pattern(rule->output_patterns[0]).AppendSubst(input, output, &o);
        input = Intern(o);
      } else if (rule->is_suffix_rule) {
        input = Intern(ReplaceSuffix(output, input));
      }

      n->actual_inputs.push_back(input);
      DepNode* c = BuildPlan(input, output, tsvs);
      n->deps.push_back(c);
    }

    // TODO: order only
    n->has_rule = true;
    n->cmds = rule->cmds;

    return n;
  }

  unordered_map<StringPiece, shared_ptr<Rule>> rules_;
  const Vars& vars_;
  const unordered_map<StringPiece, Vars*>& rule_vars_;

  vector<shared_ptr<Rule>> implicit_rules_;   // pattern=%. no prefix,suffix.
  //vector<Rule*> iprefix_rules_;   // pattern=prefix%..  may have suffix
  //vector<Rule*> isuffix_rules_;   // pattern=%suffix  no prefix
  typedef unordered_map<StringPiece, vector<shared_ptr<Rule>>> SuffixRuleMap;
  SuffixRuleMap suffix_rules_;

  shared_ptr<Rule> first_rule_;
  unordered_map<StringPiece, DepNode*> done_;
  unordered_set<StringPiece> phony_;
};

void MakeDep(const vector<shared_ptr<Rule>>& rules,
             const Vars& vars,
             const unordered_map<StringPiece, Vars*>& rule_vars,
             const vector<StringPiece>& targets,
             vector<DepNode*>* nodes) {
  DepBuilder db(rules, vars, rule_vars);
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
