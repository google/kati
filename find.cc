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

#include "find.h"

#include <dirent.h>
#include <fnmatch.h>
#include <limits.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#include <algorithm>
#include <memory>
#include <vector>

//#undef NOLOG

#include "fileutil.h"
#include "log.h"
#include "string_piece.h"
#include "strutil.h"
#include "timeutil.h"

#define FIND_WARN_LOC(...) do {         \
    if (g_flags.werror_find_emulator) { \
      ERROR_LOC(__VA_ARGS__);           \
    } else {                            \
      WARN_LOC(__VA_ARGS__);            \
    }                                   \
  } while (0)

class FindCond {
 public:
  virtual ~FindCond() = default;
  virtual bool IsTrue(const string& path, unsigned char type) const = 0;
  virtual bool Countable() const = 0;
  virtual unsigned Count() const = 0;
 protected:
  FindCond() = default;
};

namespace {

class NameCond : public FindCond {
 public:
  explicit NameCond(const string& n)
      : name_(n) {
    has_wildcard_ = (n.find_first_of("?*[") != string::npos);
  }
  virtual bool IsTrue(const string& path, unsigned char) const override {
    return fnmatch(name_.c_str(), Basename(path).data(), 0) == 0;
  }
  virtual bool Countable() const override {
    return !has_wildcard_;
  }
  virtual unsigned Count() const override {
    return 1;
  }
 private:
  string name_;
  bool has_wildcard_;
};

class TypeCond : public FindCond {
 public:
  explicit TypeCond(unsigned char t)
      : type_(t) {
  }
  virtual bool IsTrue(const string&, unsigned char type) const override {
    return type == type_;
  }
  virtual bool Countable() const override {
    return false;
  }
  virtual unsigned Count() const override {
    return 0;
  }
 private:
  unsigned char type_;
};

class NotCond : public FindCond {
 public:
  NotCond(FindCond* c)
      : c_(c) {
  }
  virtual bool IsTrue(const string& path, unsigned char type) const override {
    return !c_->IsTrue(path, type);
  }
  virtual bool Countable() const override {
    return false;
  }
  virtual unsigned Count() const override {
    return 0;
  }
 private:
  unique_ptr<FindCond> c_;
};

class AndCond : public FindCond {
 public:
  AndCond(FindCond* c1, FindCond* c2)
      : c1_(c1), c2_(c2) {
  }
  virtual bool IsTrue(const string& path, unsigned char type) const override {
    if (c1_->IsTrue(path, type))
      return c2_->IsTrue(path, type);
    return false;
  }
  virtual bool Countable() const override {
    return false;
  }
  virtual unsigned Count() const override {
    return 0;
  }
 private:
  unique_ptr<FindCond> c1_, c2_;
};

class OrCond : public FindCond {
 public:
  OrCond(FindCond* c1, FindCond* c2)
      : c1_(c1), c2_(c2) {
  }
  virtual bool IsTrue(const string& path, unsigned char type) const override {
    if (!c1_->IsTrue(path, type))
      return c2_->IsTrue(path, type);
    return true;
  }
  virtual bool Countable() const override {
    return c1_->Countable() && c2_->Countable();;
  }
  virtual unsigned Count() const override {
    return c1_->Count() + c2_->Count();
  }
 private:
  unique_ptr<FindCond> c1_, c2_;
};

class DirentNode {
 public:
  virtual ~DirentNode() = default;

  virtual const DirentNode* FindDir(StringPiece) const {
    return NULL;
  }
  virtual bool RunFind(const FindCommand& fc, const Loc& loc, int d,
                       string* path,
                       unordered_map<const DirentNode*, string>* cur_read_dirs,
                       vector<string>& out) const = 0;

  virtual bool IsDirectory() const = 0;

  const string& base() const { return base_; }

 protected:
  explicit DirentNode(const string& name) {
    base_ = Basename(name).as_string();
  }

  void PrintIfNecessary(const FindCommand& fc,
                        const string& path,
                        unsigned char type,
                        int d,
                        vector<string>& out) const {
    if (fc.print_cond && !fc.print_cond->IsTrue(path, type))
      return;
    if (d < fc.mindepth)
      return;
    out.push_back(path);
  }

  string base_;
};

class DirentFileNode : public DirentNode {
 public:
  DirentFileNode(const string& name, unsigned char type)
      : DirentNode(name), type_(type) {
  }

  virtual bool RunFind(const FindCommand& fc, const Loc&, int d,
                       string* path,
                       unordered_map<const DirentNode*, string>*,
                       vector<string>& out) const override {
    PrintIfNecessary(fc, *path, type_, d, out);
    return true;
  }

  virtual bool IsDirectory() const override { return false; }

 private:
  unsigned char type_;
};

struct ScopedReadDirTracker {
 public:
  ScopedReadDirTracker(const DirentNode* n,
                       const string& path,
                       unordered_map<const DirentNode*, string>* cur_read_dirs)
      : n_(NULL), cur_read_dirs_(cur_read_dirs) {
    const auto& p = cur_read_dirs->emplace(n, path);
    if (p.second) {
      n_ = n;
    } else {
      conflicted_ = p.first->second;
    }
  }

  ~ScopedReadDirTracker() {
    if (n_)
      cur_read_dirs_->erase(n_);
  }

  bool ok() const { return conflicted_.empty(); }
  const string& conflicted() const { return conflicted_; }

 private:
  string conflicted_;
  const DirentNode* n_;
  unordered_map<const DirentNode*, string>* cur_read_dirs_;
};

class DirentDirNode : public DirentNode {
 public:
  explicit DirentDirNode(const string& name)
      : DirentNode(name) {
  }

  ~DirentDirNode() {
    for (auto& p : children_) {
      delete p.second;
    }
  }

  virtual const DirentNode* FindDir(StringPiece d) const override {
    if (d.empty() || d == ".")
      return this;
    size_t index = d.find('/');
    const string& p = d.substr(0, index).as_string();
    for (auto& child : children_) {
      if (p == child.first) {
        if (index == string::npos)
          return child.second;
        StringPiece nd = d.substr(index + 1);
        return child.second->FindDir(nd);
      }
    }
    return NULL;
  }

  virtual bool RunFind(const FindCommand& fc, const Loc& loc, int d,
                       string* path,
                       unordered_map<const DirentNode*, string>* cur_read_dirs,
                       vector<string>& out) const override {
    ScopedReadDirTracker srdt(this, *path, cur_read_dirs);
    if (!srdt.ok()) {
      FIND_WARN_LOC(loc, "FindEmulator: find: File system loop detected; `%s' "
                    "is part of the same file system loop as `%s'.",
                    path->c_str(), srdt.conflicted().c_str());
      return true;
    }

    fc.read_dirs->insert(*path);

    if (fc.prune_cond && fc.prune_cond->IsTrue(*path, DT_DIR)) {
      if (fc.type != FindCommandType::FINDLEAVES) {
        out.push_back(*path);
      }
      return true;
    }

    PrintIfNecessary(fc, *path, DT_DIR, d, out);

    if (d >= fc.depth)
      return true;

    size_t orig_path_size = path->size();
    if (fc.type == FindCommandType::FINDLEAVES) {
      size_t orig_out_size = out.size();
      for (const auto& p : children_) {
        DirentNode* c = p.second;
        // We will handle directories later.
        if (c->IsDirectory())
          continue;
        if ((*path)[path->size()-1] != '/')
          *path += '/';
        *path += c->base();
        if (!c->RunFind(fc, loc, d + 1, path, cur_read_dirs, out))
          return false;
        path->resize(orig_path_size);
      }

      // Found a leaf, stop the search.
      if (orig_out_size != out.size()) {
        // If we've found all possible files in this directory, we don't need
        // to add a regen dependency on the directory, we just need to ensure
        // that the files are not removed.
        if (fc.print_cond->Countable() &&
            fc.print_cond->Count() == out.size() - orig_out_size) {
          fc.read_dirs->erase(*path);
          for (unsigned i = orig_out_size; i < out.size(); i++) {
            fc.found_files->push_back(out[i]);
          }
        }

        return true;
      }

      for (const auto& p : children_) {
        DirentNode* c = p.second;
        if (!c->IsDirectory())
          continue;
        if ((*path)[path->size()-1] != '/')
          *path += '/';
        *path += c->base();
        if (!c->RunFind(fc, loc, d + 1, path, cur_read_dirs, out))
          return false;
        path->resize(orig_path_size);
      }
    } else {
      for (const auto& p : children_) {
        DirentNode* c = p.second;
        if ((*path)[path->size()-1] != '/')
          *path += '/';
        *path += c->base();
        if (!c->RunFind(fc, loc, d + 1, path, cur_read_dirs, out))
          return false;
        path->resize(orig_path_size);
      }
    }
    return true;
  }

  virtual bool IsDirectory() const override { return true; }

  void Add(const string& name, DirentNode* c) {
    children_.emplace(children_.end(), name, c);
  }

 private:
  vector<pair<string, DirentNode*>> children_;
};

class DirentSymlinkNode : public DirentNode {
 public:
  explicit DirentSymlinkNode(const string& name)
      : DirentNode(name), to_(NULL), errno_(0) {
  }

  virtual const DirentNode* FindDir(StringPiece d) const override {
    if (errno_ == 0 && to_)
      return to_->FindDir(d);
    return NULL;
  }

  virtual bool RunFind(const FindCommand& fc, const Loc& loc, int d,
                       string* path,
                       unordered_map<const DirentNode*, string>* cur_read_dirs,
                       vector<string>& out) const override {
    unsigned char type = DT_LNK;
    if (fc.follows_symlinks && errno_ != ENOENT) {
      if (errno_) {
        if (fc.type != FindCommandType::FINDLEAVES) {
          FIND_WARN_LOC(loc, "FindEmulator: find: `%s': %s",
                        path->c_str(), strerror(errno_));
        }
        return true;
      }

      if (!to_) {
        LOG("FindEmulator does not support %s", path->c_str());
        return false;
      }

      return to_->RunFind(fc, loc, d, path, cur_read_dirs, out);
    }
    PrintIfNecessary(fc, *path, type, d, out);
    return true;
  }

  virtual bool IsDirectory() const override {
    return errno_ == 0 && to_ && to_->IsDirectory();
  }

  void set_to(const DirentNode* to) {
    to_ = to;
  }

  void set_errno(int e) {
    errno_ = e;
  }

 private:
  const DirentNode* to_;
  int errno_;
};

class FindCommandParser {
 public:
  FindCommandParser(StringPiece cmd, FindCommand* fc)
      : cmd_(cmd), fc_(fc), has_if_(false) {
  }

  bool Parse() {
    cur_ = cmd_;
    if (!ParseImpl()) {
      LOG("FindEmulator: Unsupported find command: %.*s", SPF(cmd_));
      return false;
    }
    CHECK(TrimLeftSpace(cur_).empty());
    return true;
  }

 private:
  bool GetNextToken(StringPiece* tok) {
    if (!unget_tok_.empty()) {
      *tok = unget_tok_;
      unget_tok_.clear();
      return true;
    }

    cur_ = TrimLeftSpace(cur_);

    if (cur_[0] == ';') {
      *tok = cur_.substr(0, 1);
      cur_ = cur_.substr(1);
      return true;
    }
    if (cur_[0] == '&') {
      if (cur_.get(1) != '&') {
        return false;
      }
      *tok = cur_.substr(0, 2);
      cur_ = cur_.substr(2);
      return true;
    }

    size_t i = 0;
    while (i < cur_.size() && !isspace(cur_[i]) &&
           cur_[i] != ';' && cur_[i] != '&') {
      i++;
    }

    *tok = cur_.substr(0, i);
    cur_ = cur_.substr(i);

    const char c = tok->get(0);
    if (c == '\'' || c == '"') {
      if (tok->size() < 2 || (*tok)[tok->size()-1] != c)
        return false;
      *tok = tok->substr(1, tok->size() - 2);
      return true;
    }

    return true;
  }

  void UngetToken(StringPiece tok) {
    CHECK(unget_tok_.empty());
    if (!tok.empty())
      unget_tok_ = tok;
  }

  bool ParseTest() {
    if (has_if_ || !fc_->testdir.empty())
      return false;
    StringPiece tok;
    if (!GetNextToken(&tok) || tok != "-d")
      return false;
    if (!GetNextToken(&tok) || tok.empty())
      return false;
    fc_->testdir = tok.as_string();
    return true;
  }

  FindCond* ParseFact(StringPiece tok) {
    if (tok == "-not" || tok == "\\!") {
      if (!GetNextToken(&tok) || tok.empty())
        return NULL;
      unique_ptr<FindCond> c(ParseFact(tok));
      if (!c.get())
        return NULL;
      return new NotCond(c.release());
    } else if (tok == "\\(") {
      if (!GetNextToken(&tok) || tok.empty())
        return NULL;
      unique_ptr<FindCond> c(ParseExpr(tok));
      if (!GetNextToken(&tok) || tok != "\\)") {
        return NULL;
      }
      return c.release();
    } else if (tok == "-name") {
      if (!GetNextToken(&tok) || tok.empty())
        return NULL;
      return new NameCond(tok.as_string());
    } else if (tok == "-type") {
      if (!GetNextToken(&tok) || tok.empty())
        return NULL;
      char type;
      if (tok == "b")
        type = DT_BLK;
      else if (tok == "c")
        type = DT_CHR;
      else if (tok == "d")
        type = DT_DIR;
      else if (tok == "p")
        type = DT_FIFO;
      else if (tok == "l")
        type = DT_LNK;
      else if (tok == "f")
        type = DT_REG;
      else if (tok == "s")
        type = DT_SOCK;
      else
        return NULL;
      return new TypeCond(type);
    } else {
      UngetToken(tok);
      return NULL;
    }
  }

  FindCond* ParseTerm(StringPiece tok) {
    unique_ptr<FindCond> c(ParseFact(tok));
    if (!c.get())
      return NULL;
    while (true) {
      if (!GetNextToken(&tok))
        return NULL;
      if (tok != "-and" && tok != "-a") {
        UngetToken(tok);
        return c.release();
      }
      if (!GetNextToken(&tok) || tok.empty())
        return NULL;
      unique_ptr<FindCond> r(ParseFact(tok));
      if (!r.get()) {
        return NULL;
      }
      c.reset(new AndCond(c.release(), r.release()));
    }
  }

  FindCond* ParseExpr(StringPiece tok) {
    unique_ptr<FindCond> c(ParseTerm(tok));
    if (!c.get())
      return NULL;
    while (true) {
      if (!GetNextToken(&tok))
        return NULL;
      if (tok != "-or" && tok != "-o") {
        UngetToken(tok);
        return c.release();
      }
      if (!GetNextToken(&tok) || tok.empty())
        return NULL;
      unique_ptr<FindCond> r(ParseTerm(tok));
      if (!r.get()) {
        return NULL;
      }
      c.reset(new OrCond(c.release(), r.release()));
    }
  }

  // <expr> ::= <term> {<or> <term>}
  // <term> ::= <fact> {<and> <fact>}
  // <fact> ::= <not> <fact> | '\(' <expr> '\)' | <pred>
  // <not> ::= '-not' | '\!'
  // <and> ::= '-and' | '-a'
  // <or> ::= '-or' | '-o'
  // <pred> ::= <name> | <type> | <maxdepth>
  // <name> ::= '-name' NAME
  // <type> ::= '-type' TYPE
  // <maxdepth> ::= '-maxdepth' MAXDEPTH
  FindCond* ParseFindCond(StringPiece tok) {
    return ParseExpr(tok);
  }

  bool ParseFind() {
    fc_->type = FindCommandType::FIND;
    StringPiece tok;
    while (true) {
      if (!GetNextToken(&tok))
        return false;
      if (tok.empty() || tok == ";")
        return true;

      if (tok == "-L") {
        fc_->follows_symlinks = true;
      } else if (tok == "-prune") {
        if (!fc_->print_cond || fc_->prune_cond)
          return false;
        if (!GetNextToken(&tok) || tok != "-o")
          return false;
        fc_->prune_cond.reset(fc_->print_cond.release());
      } else if (tok == "-print") {
        if (!GetNextToken(&tok) || !tok.empty())
          return false;
        return true;
      } else if (tok == "-maxdepth") {
        if (!GetNextToken(&tok) || tok.empty())
          return false;
        const string& depth_str = tok.as_string();
        char* endptr;
        long d = strtol(depth_str.c_str(), &endptr, 10);
        if (endptr != depth_str.data() + depth_str.size() ||
            d < 0 || d > INT_MAX) {
          return false;
        }
        fc_->depth = d;
      } else if (tok[0] == '-' || tok == "\\(") {
        if (fc_->print_cond.get())
          return false;
        FindCond* c = ParseFindCond(tok);
        if (!c)
          return false;
        fc_->print_cond.reset(c);
      } else if (tok == "2>") {
        if (!GetNextToken(&tok) || tok != "/dev/null") {
          return false;
        }
        fc_->redirect_to_devnull = true;
      } else if (tok.find_first_of("|;&><*'\"") != string::npos) {
        return false;
      } else {
        fc_->finddirs.push_back(tok.as_string());
      }
    }
  }

  bool ParseFindLeaves() {
    fc_->type = FindCommandType::FINDLEAVES;
    fc_->follows_symlinks = true;
    StringPiece tok;
    vector<string> findfiles;
    while (true) {
      if (!GetNextToken(&tok))
        return false;
      if (tok.empty()) {
        if (fc_->finddirs.size() == 0) {
          // backwards compatibility
          if (findfiles.size() < 2)
            return false;
          fc_->finddirs.swap(findfiles);
          fc_->print_cond.reset(new NameCond(fc_->finddirs.back()));
          fc_->finddirs.pop_back();
        } else {
          if (findfiles.size() < 1)
            return false;
          for (auto& file : findfiles) {
            FindCond* cond = new NameCond(file);
            if (fc_->print_cond.get()) {
              cond = new OrCond(fc_->print_cond.release(), cond);
            }
            CHECK(!fc_->print_cond.get());
            fc_->print_cond.reset(cond);
          }
        }
        return true;
      }

      if (HasPrefix(tok, "--prune=")) {
        FindCond* cond = new NameCond(
            tok.substr(strlen("--prune=")).as_string());
        if (fc_->prune_cond.get()) {
          cond = new OrCond(fc_->prune_cond.release(), cond);
        }
        CHECK(!fc_->prune_cond.get());
        fc_->prune_cond.reset(cond);
      } else if (HasPrefix(tok, "--mindepth=")) {
        string mindepth_str = tok.substr(strlen("--mindepth=")).as_string();
        char* endptr;
        long d = strtol(mindepth_str.c_str(), &endptr, 10);
        if (endptr != mindepth_str.data() + mindepth_str.size() ||
            d < INT_MIN || d > INT_MAX) {
          return false;
        }
        fc_->mindepth = d;
      } else if (HasPrefix(tok, "--dir=")) {
        StringPiece dir= tok.substr(strlen("--dir="));
        fc_->finddirs.push_back(dir.as_string());
      } else if (HasPrefix(tok, "--")) {
        if (g_flags.werror_find_emulator) {
          ERROR("Unknown flag in findleaves.py: %.*s", SPF(tok));
        } else {
          WARN("Unknown flag in findleaves.py: %.*s", SPF(tok));
        }
        return false;
      } else {
        findfiles.push_back(tok.as_string());
      }
    }
  }

  bool ParseImpl() {
    while (true) {
      StringPiece tok;
      if (!GetNextToken(&tok))
        return false;

      if (tok.empty())
        return true;

      if (tok == "cd") {
        if (!GetNextToken(&tok) || tok.empty() || !fc_->chdir.empty())
          return false;
        fc_->chdir = tok.as_string();
        if (!GetNextToken(&tok) || (tok != ";" && tok != "&&"))
          return false;
      } else if (tok == "if") {
        if (!GetNextToken(&tok) || tok != "[")
          return false;
        if (!ParseTest())
          return false;
        if (!GetNextToken(&tok) || tok != "]")
          return false;
        if (!GetNextToken(&tok) || tok != ";")
          return false;
        if (!GetNextToken(&tok) || tok != "then")
          return false;
        has_if_ = true;
      } else if (tok == "test") {
        if (!fc_->chdir.empty())
          return false;
        if (!ParseTest())
          return false;
        if (!GetNextToken(&tok) || tok != "&&")
          return false;
      } else if (tok == "find") {
        if (!ParseFind())
          return false;
        if (has_if_) {
          if (!GetNextToken(&tok) || tok != "fi")
            return false;
        }
        if (!GetNextToken(&tok) || !tok.empty())
          return false;
        return true;
      } else if (tok == "build/tools/findleaves.py") {
        if (!ParseFindLeaves())
          return false;
        return true;
      } else {
        return false;
      }
    }
  }

  StringPiece cmd_;
  StringPiece cur_;
  FindCommand* fc_;
  bool has_if_;
  StringPiece unget_tok_;
};

static FindEmulator* g_instance;

class FindEmulatorImpl : public FindEmulator {
 public:
  FindEmulatorImpl()
      : node_cnt_(0), is_initialized_(false) {
    g_instance = this;
  }

  virtual ~FindEmulatorImpl() = default;

  bool CanHandle(StringPiece s) const {
    return (!HasPrefix(s, "../") &&
            !HasPrefix(s, "/") &&
            !HasPrefix(s, ".repo") &&
            !HasPrefix(s, ".git"));
  }

  const DirentNode* FindDir(StringPiece d, bool* should_fallback) {
    const DirentNode* r = root_->FindDir(d);
    if (!r) {
      *should_fallback = Exists(d);
    }
    return r;
  }

  virtual bool HandleFind(const string& cmd UNUSED, const FindCommand& fc,
                          const Loc& loc, string* out) override {
    if (!CanHandle(fc.chdir)) {
      LOG("FindEmulator: Cannot handle chdir (%.*s): %s",
          SPF(fc.chdir), cmd.c_str());
      return false;
    }

    if (!is_initialized_) {
      ScopedTimeReporter tr("init find emulator time");
      root_.reset(ConstructDirectoryTree(""));
      if (!root_) {
        ERROR("FindEmulator: Cannot open root directory");
      }
      ResolveSymlinks();
      LOG_STAT("%d find nodes", node_cnt_);
      is_initialized_ = true;
    }

    if (!fc.testdir.empty()) {
      if (!CanHandle(fc.testdir)) {
        LOG("FindEmulator: Cannot handle test dir (%.*s): %s",
            SPF(fc.testdir), cmd.c_str());
        return false;
      }
      bool should_fallback = false;
      if (!FindDir(fc.testdir, &should_fallback)) {
        LOG("FindEmulator: Test dir (%.*s) not found: %s",
            SPF(fc.testdir), cmd.c_str());
        return !should_fallback;
      }
    }

    if (!fc.chdir.empty()) {
      if (!CanHandle(fc.chdir)) {
        LOG("FindEmulator: Cannot handle chdir (%.*s): %s",
            SPF(fc.chdir), cmd.c_str());
        return false;
      }
      bool should_fallback = false;
      if (!FindDir(fc.chdir, &should_fallback)) {
        if (should_fallback)
          return false;
        if (!fc.redirect_to_devnull) {
          FIND_WARN_LOC(loc, "FindEmulator: cd: %.*s: No such file or directory",
                        SPF(fc.chdir));
        }
        return true;
      }
    }

    vector<string> results;
    for (const string& finddir : fc.finddirs) {
      const string dir = ConcatDir(fc.chdir, finddir);

      if (!CanHandle(dir)) {
        LOG("FindEmulator: Cannot handle find dir (%s): %s",
            dir.c_str(), cmd.c_str());
        return false;
      }

      bool should_fallback = false;
      const DirentNode* base = FindDir(dir, &should_fallback);
      if (!base) {
        if (should_fallback) {
          return false;
        }
        if (!fc.redirect_to_devnull) {
          FIND_WARN_LOC(loc, "FindEmulator: find: `%s': No such file or directory",
                        ConcatDir(fc.chdir, finddir).c_str());
        }
        continue;
      }

      string path = finddir;
      unordered_map<const DirentNode*, string> cur_read_dirs;
      if (!base->RunFind(fc, loc, 0, &path, &cur_read_dirs, results)) {
        LOG("FindEmulator: RunFind failed: %s", cmd.c_str());
        return false;
      }
    }

    if (results.size() > 0) {
      // Calculate and reserve necessary space in out
      size_t new_length = 0;
      for (const string& result : results) {
        new_length += result.size() + 1;
      }
      out->reserve(out->size() + new_length - 1);

      if (fc.type == FindCommandType::FINDLEAVES) {
        sort(results.begin(), results.end());
      }

      WordWriter writer(out);
      for (const string& result : results) {
        writer.Write(result);
      }
    }

    LOG("FindEmulator: OK");
    return true;
  }

 private:
  static unsigned char GetDtTypeFromStat(const struct stat& st) {
    if (S_ISREG(st.st_mode)) {
      return DT_REG;
    } else if (S_ISDIR(st.st_mode)) {
      return DT_DIR;
    } else if (S_ISCHR(st.st_mode)) {
      return DT_CHR;
    } else if (S_ISBLK(st.st_mode)) {
      return DT_BLK;
    } else if (S_ISFIFO(st.st_mode)) {
      return DT_FIFO;
    } else if (S_ISLNK(st.st_mode)) {
      return DT_LNK;
    } else if (S_ISSOCK(st.st_mode)) {
      return DT_SOCK;
    } else {
      return DT_UNKNOWN;
    }
  }

  static unsigned char GetDtType(const string& path) {
    struct stat st;
    if (lstat(path.c_str(), &st)) {
      PERROR("stat for %s", path.c_str());
    }
    return GetDtTypeFromStat(st);
  }

  DirentNode* ConstructDirectoryTree(const string& path) {
    DIR* dir = opendir(path.empty() ? "." : path.c_str());
    if (!dir) {
      if (errno == ENOENT || errno == EACCES) {
        LOG("opendir failed: %s", path.c_str());
        return NULL;
      } else {
        PERROR("opendir failed: %s", path.c_str());
      }
    }

    DirentDirNode* n = new DirentDirNode(path);

    struct dirent* ent;
    while ((ent = readdir(dir)) != NULL) {
      if (!strcmp(ent->d_name, ".") ||
          !strcmp(ent->d_name, "..") ||
          !strcmp(ent->d_name, ".repo") ||
          !strcmp(ent->d_name, ".git"))
        continue;

      string npath = path;
      if (!path.empty())
        npath += '/';
      npath += ent->d_name;

      DirentNode* c = NULL;
      auto d_type = ent->d_type;
      if (d_type == DT_UNKNOWN) {
        d_type = GetDtType(npath);
        CHECK(d_type != DT_UNKNOWN);
      }
      if (d_type == DT_DIR) {
        c = ConstructDirectoryTree(npath);
        if (c == NULL) {
          continue;
        }
      } else if (d_type == DT_LNK) {
        auto s = new DirentSymlinkNode(npath);
        symlinks_.push_back(make_pair(npath, s));
        c = s;
      } else {
        c = new DirentFileNode(npath, d_type);
      }
      node_cnt_++;
      n->Add(ent->d_name, c);
    }
    closedir(dir);

    return n;
  }

  void ResolveSymlinks() {
    vector<pair<string, DirentSymlinkNode*>> symlinks;
    symlinks.swap(symlinks_);
    for (const auto& p : symlinks) {
      const string& path = p.first;
      DirentSymlinkNode* s = p.second;

      char buf[PATH_MAX+1];
      buf[PATH_MAX] = 0;
      ssize_t len = readlink(path.c_str(), buf, PATH_MAX);
      if (len < 0) {
        WARN("readlink failed: %s", path.c_str());
        continue;
      }
      buf[len] = 0;

      struct stat st;
      unsigned char type = DT_UNKNOWN;
      if (stat(path.c_str(), &st) == 0) {
        type = GetDtTypeFromStat(st);
      } else {
        s->set_errno(errno);
        LOG("stat failed: %s: %s", path.c_str(), strerror(errno));
      }

      if (*buf != '/') {
        const string npath = ConcatDir(Dirname(path), buf);
        bool should_fallback = false;
        const DirentNode* to = FindDir(npath, &should_fallback);
        if (to) {
          s->set_to(to);
          continue;
        }
      }

      if (type == DT_DIR) {
        if (path.find('/') == string::npos) {
          DirentNode* dir = ConstructDirectoryTree(path);
          if (dir != NULL) {
            s->set_to(dir);
          } else {
            s->set_errno(errno);
          }
        }
      } else if (type != DT_LNK && type != DT_UNKNOWN) {
          s->set_to(new DirentFileNode(path, type));
      }
    }

    if (!symlinks_.empty())
      ResolveSymlinks();
  }

  unique_ptr<DirentNode> root_;
  vector<pair<string, DirentSymlinkNode*>> symlinks_;
  int node_cnt_;
  bool is_initialized_;
};

}  // namespace

FindCommand::FindCommand()
    : follows_symlinks(false), depth(INT_MAX), mindepth(INT_MIN),
      redirect_to_devnull(false),
      found_files(new vector<string>()),
      read_dirs(new unordered_set<string>()) {
}

FindCommand::~FindCommand() {
}

bool FindCommand::Parse(const string& cmd) {
  FindCommandParser fcp(cmd, this);
  if (!HasWord(cmd, "find") && !HasWord(cmd, "build/tools/findleaves.py"))
    return false;

  if (!fcp.Parse())
    return false;

  NormalizePath(&chdir);
  NormalizePath(&testdir);
  if (finddirs.empty())
    finddirs.push_back(".");
  return true;
}

FindEmulator* FindEmulator::Get() {
  return g_instance;
}

void InitFindEmulator() {
  new FindEmulatorImpl();
}
