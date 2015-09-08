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

#include <memory>
#include <vector>

//#undef NOLOG

#include "log.h"
#include "string_piece.h"
#include "strutil.h"
#include "timeutil.h"

class FindCond {
 public:
  virtual ~FindCond() = default;
  virtual bool IsTrue(const string& name, unsigned char type) const = 0;
 protected:
  FindCond() = default;
};

namespace {

class NameCond : public FindCond {
 public:
  explicit NameCond(const string& n)
      : name_(n) {
  }
  virtual bool IsTrue(const string& name, unsigned char) const {
    return fnmatch(name_.c_str(), name.c_str(), 0) == 0;
  }
 private:
  string name_;
};

class TypeCond : public FindCond {
 public:
  explicit TypeCond(unsigned char t)
      : type_(t) {
  }
  virtual bool IsTrue(const string&, unsigned char type) const {
    return type == type_;
  }
 private:
  unsigned char type_;
};

class NotCond : public FindCond {
 public:
  NotCond(FindCond* c)
      : c_(c) {
  }
  virtual bool IsTrue(const string& name, unsigned char type) const {
    return !c_->IsTrue(name, type);
  }
 private:
  unique_ptr<FindCond> c_;
};

class AndCond : public FindCond {
 public:
  AndCond(FindCond* c1, FindCond* c2)
      : c1_(c1), c2_(c2) {
  }
  virtual bool IsTrue(const string& name, unsigned char type) const {
    if (c1_->IsTrue(name, type))
      return c2_->IsTrue(name, type);
    return false;
  }
 private:
  unique_ptr<FindCond> c1_, c2_;
};

class OrCond : public FindCond {
 public:
  OrCond(FindCond* c1, FindCond* c2)
      : c1_(c1), c2_(c2) {
  }
  virtual bool IsTrue(const string& name, unsigned char type) const {
    if (!c1_->IsTrue(name, type))
      return c2_->IsTrue(name, type);
    return true;
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
  virtual bool RunFind(const FindCommand& fc, int d,
                       string* path, string* out) const = 0;

  const string& base() const { return base_; }

 protected:
  explicit DirentNode(const string& name) {
    base_ = Basename(name).as_string();
  }

  void PrintIfNecessary(const FindCommand& fc,
                        const string& path,
                        unsigned char type,
                        int d,
                        string* out) const {
    if (fc.print_cond && !fc.print_cond->IsTrue(base_, type))
      return;
    if (d < fc.mindepth)
      return;
    *out += path;
    *out += ' ';
  }

  string base_;
};

class DirentFileNode : public DirentNode {
 public:
  DirentFileNode(const string& name, unsigned char type)
      : DirentNode(name), type_(type) {
  }

  virtual bool RunFind(const FindCommand& fc, int d,
                       string* path, string* out) const {
    PrintIfNecessary(fc, *path, type_, d, out);
    return true;
  }

 private:
  unsigned char type_;
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

  virtual const DirentNode* FindDir(StringPiece d) const {
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

  virtual bool RunFind(const FindCommand& fc, int d,
                       string* path, string* out) const {
    fc.read_dirs->push_back(*path);

    if (fc.prune_cond && fc.prune_cond->IsTrue(base_, DT_DIR)) {
      if (fc.type != FindCommandType::FINDLEAVES) {
        *out += *path;
        *out += ' ';
      }
      return true;
    }

    PrintIfNecessary(fc, *path, DT_DIR, d, out);

    if (d >= fc.depth)
      return true;

    size_t orig_path_size = path->size();
    if (fc.type == FindCommandType::FINDLEAVES) {
      size_t orig_out_size = out->size();
      for (const auto& p : children_) {
        DirentNode* c = p.second;
        // We will handle directories later.
        if (dynamic_cast<DirentDirNode*>(c))
          continue;
        if ((*path)[path->size()-1] != '/')
          *path += '/';
        *path += c->base();
        if (!c->RunFind(fc, d + 1, path, out))
          return false;
        path->resize(orig_path_size);
        // Found a leaf, stop the search.
        if (orig_out_size != out->size())
          return true;
      }

      for (const auto& p : children_) {
        DirentNode* c = p.second;
        if (!dynamic_cast<DirentDirNode*>(c))
          continue;
        if ((*path)[path->size()-1] != '/')
          *path += '/';
        *path += c->base();
        if (!c->RunFind(fc, d + 1, path, out))
          return false;
        path->resize(orig_path_size);
      }
    } else {
      for (const auto& p : children_) {
        DirentNode* c = p.second;
        if ((*path)[path->size()-1] != '/')
          *path += '/';
        *path += c->base();
        if (!c->RunFind(fc, d + 1, path, out))
          return false;
        path->resize(orig_path_size);
      }
    }
    return true;
  }

  void Add(const string& name, DirentNode* c) {
    children_.emplace(children_.end(), name, c);
  }

 private:
  vector<pair<string, DirentNode*>> children_;
};

class DirentSymlinkNode : public DirentNode {
 public:
  explicit DirentSymlinkNode(const string& name)
      : DirentNode(name) {
  }

  virtual bool RunFind(const FindCommand& fc, int d,
                       string* path, string* out) const {
    unsigned char type = DT_LNK;
    if (fc.follows_symlinks) {
      // TODO
      LOG("FindEmulator: symlink is hard");
      return false;

      char buf[PATH_MAX+1];
      buf[PATH_MAX] = 0;
      LOG("path=%s", path->c_str());
      ssize_t len = readlink(path->c_str(), buf, PATH_MAX);
      if (len > 0) {
        buf[len] = 0;
        string oldpath;
        if (buf[0] != '/') {
          Dirname(*path).AppendToString(&oldpath);
          oldpath += '/';
        }
        oldpath += buf;

        LOG("buf=%s old=%s", buf, oldpath.c_str());

        struct stat st;
        if (stat(oldpath.c_str(), &st) == 0) {
          LOG("st OK");
          if (S_ISREG(st.st_mode)) {
            type = DT_REG;
          } else if (S_ISDIR(st.st_mode)) {
            type = DT_DIR;
          } else if (S_ISCHR(st.st_mode)) {
            type = DT_CHR;
          } else if (S_ISBLK(st.st_mode)) {
            type = DT_BLK;
          } else if (S_ISFIFO(st.st_mode)) {
            type = DT_FIFO;
          } else if (S_ISLNK(st.st_mode)) {
            type = DT_LNK;
          } else if (S_ISSOCK(st.st_mode)) {
            type = DT_SOCK;
          } else {
            return false;
          }
        }
      }
    }
    PrintIfNecessary(fc, *path, type, d, out);
    return true;
  }
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
        fc_->finddirs.push_back(tok);
      }
    }
  }

  bool ParseFindLeaves() {
    fc_->type = FindCommandType::FINDLEAVES;
    StringPiece tok;
    while (true) {
      if (!GetNextToken(&tok))
        return false;
      if (tok.empty()) {
        if (fc_->finddirs.size() < 2)
          return false;
        fc_->print_cond.reset(new NameCond(fc_->finddirs.back().as_string()));
        fc_->finddirs.pop_back();
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
      } else if (HasPrefix(tok, "--")) {
        WARN("Unknown flag in findleaves.py: %.*s", SPF(tok));
        return false;
      } else {
        fc_->finddirs.push_back(tok);
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
            !HasPrefix(s, ".git") &&
            !HasPrefix(s, "out"));
  }

  virtual bool HandleFind(const string& cmd UNUSED, const FindCommand& fc,
                          string* out) override {
    if (!CanHandle(fc.chdir)) {
      LOG("FindEmulator: Cannot handle chdir (%.*s): %s",
          SPF(fc.chdir), cmd.c_str());
      return false;
    }

    if (!is_initialized_) {
      ScopedTimeReporter tr("init find emulator time");
      root_.reset(ConstructDirectoryTree(""));
      LOG_STAT("%d find nodes", node_cnt_);
      is_initialized_ = true;
    }

    if (!fc.testdir.empty()) {
      if (!CanHandle(fc.testdir)) {
        LOG("FindEmulator: Cannot handle test dir (%.*s): %s",
            SPF(fc.testdir), cmd.c_str());
        return false;
      }
      if (!root_->FindDir(fc.testdir)) {
        LOG("FindEmulator: Test dir (%.*s) not found: %s",
            SPF(fc.testdir), cmd.c_str());
        return true;
      }
    }

    if (!fc.chdir.empty()) {
      if (!CanHandle(fc.chdir)) {
        LOG("FindEmulator: Cannot handle chdir (%.*s): %s",
            SPF(fc.chdir), cmd.c_str());
        return false;
      }
      if (!root_->FindDir(fc.chdir)) {
        if (!fc.redirect_to_devnull) {
          fprintf(stderr,
                  "FindEmulator: cd: %.*s: No such file or directory\n",
                  SPF(fc.chdir));
        }
        return true;
      }
    }

    const size_t orig_out_size = out->size();
    for (StringPiece finddir : fc.finddirs) {
      const string dir = ConcatDir(fc.chdir, finddir);

      if (!CanHandle(dir)) {
        LOG("FindEmulator: Cannot handle find dir (%s): %s",
            dir.c_str(), cmd.c_str());
        out->resize(orig_out_size);
        return false;
      }

      const DirentNode* base = root_->FindDir(dir);
      if (!base) {
        if (!fc.redirect_to_devnull) {
          fprintf(stderr,
                  "FindEmulator: find: `%s': No such file or directory\n",
                  ConcatDir(fc.chdir, finddir).c_str());
        }
        continue;
      }

      string path = finddir.as_string();
      if (!base->RunFind(fc, 0, &path, out)) {
        LOG("FindEmulator: RunFind failed: %s", cmd.c_str());
        out->resize(orig_out_size);
        return false;
      }
    }

    if (!out->empty() && (*out)[out->size()-1] == ' ')
      out->resize(out->size()-1);

    if (fc.type == FindCommandType::FINDLEAVES) {
      *out = SortWordsInString(*out);
    }

    LOG("FindEmulator: OK");
    return true;
  }

 private:
  static unsigned char GetDtType(const string& path) {
    struct stat st;
    if (lstat(path.c_str(), &st)) {
      PERROR("stat for %s", path.c_str());
    }

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

  DirentNode* ConstructDirectoryTree(const string& path) {
    DIR* dir = opendir(path.empty() ? "." : path.c_str());
    if (!dir)
      PERROR("opendir failed: %s", path.c_str());

    DirentDirNode* n = new DirentDirNode(path);

    struct dirent* ent;
    while ((ent = readdir(dir)) != NULL) {
      if (!strcmp(ent->d_name, ".") ||
          !strcmp(ent->d_name, "..") ||
          !strcmp(ent->d_name, ".repo") ||
          !strcmp(ent->d_name, ".git") ||
          !strcmp(ent->d_name, "out"))
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
      } else if (d_type == DT_LNK) {
        c = new DirentSymlinkNode(npath);
      } else {
        c = new DirentFileNode(npath, d_type);
      }
      node_cnt_++;
      n->Add(ent->d_name, c);
    }
    closedir(dir);

    return n;
  }

  unique_ptr<DirentNode> root_;
  int node_cnt_;
  bool is_initialized_;
};

}  // namespace

FindCommand::FindCommand()
    : follows_symlinks(false), depth(INT_MAX), mindepth(INT_MIN),
      redirect_to_devnull(false),
      read_dirs(new vector<string>()) {
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
