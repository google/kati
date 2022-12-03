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

#ifndef SYMTAB_H_
#define SYMTAB_H_

#include <bitset>
#include <functional>
#include <string>
#include <string_view>
#include <vector>

extern std::vector<std::string*>* g_symbols;

class Symtab;
class Var;

class Symbol {
 public:
  explicit Symbol() : v_(-1) {}

  const std::string& str() const { return *((*g_symbols)[v_]); }

  const char* c_str() const { return str().c_str(); }

  bool empty() const { return !v_; }

  int val() const { return v_; }

  char get(size_t i) const {
    const std::string& s = str();
    if (i >= s.size())
      return 0;
    return s[i];
  }

  bool IsValid() const { return v_ >= 0; }

  Var* PeekGlobalVar() const;
  Var* GetGlobalVar() const;
  void SetGlobalVar(Var* v,
                    bool is_override = false,
                    bool* readonly = nullptr) const;

 private:
  explicit Symbol(int v);

  int v_;

  friend class Symtab;
  friend class SymbolSet;
};

/* A set of symbols represented as bitmap indexed by Symbol's ordinal value. */
class SymbolSet {
 public:
  SymbolSet() : low_(0), high_(0) {}

  /* Returns true if Symbol belongs to this set. */
  bool exists(Symbol sym) const {
    size_t bit_nr = static_cast<size_t>(sym.val());
    return sym.IsValid() && bit_nr >= low_ && bit_nr < high_ &&
           bits_[(bit_nr - low_) / 64][(bit_nr - low_) % 64];
  }

  /* Adds Symbol to this set.  */
  void insert(Symbol sym) {
    if (!sym.IsValid()) {
      return;
    }
    size_t bit_nr = static_cast<size_t>(sym.val());
    if (bit_nr < low_ || bit_nr >= high_) {
      resize(bit_nr);
    }
    bits_[(bit_nr - low_) / 64][(bit_nr - low_) % 64] = true;
  }

  /* Returns the number of Symbol's in this set.  */
  size_t size() const {
    size_t n = 0;
    for (auto const& bitset : bits_) {
      n += bitset.count();
    }
    return n;
  }

  /* Allow using foreach.
   * E.g.,
   *   SymbolSet symbol_set;
   *   for (auto const& symbol: symbol_set) { ... }
   */
  class iterator {
    const SymbolSet* bitset_;
    size_t pos_;

    iterator(const SymbolSet* bitset, size_t pos)
        : bitset_(bitset), pos_(pos) {}

    /* Proceed to the next Symbol.  */
    void next() {
      size_t bit_nr = (pos_ > bitset_->low_) ? pos_ - bitset_->low_ : 0;
      while (bit_nr < (bitset_->high_ - bitset_->low_)) {
        if ((bit_nr % 64) == 0 && !bitset_->bits_[bit_nr / 64].any()) {
          bit_nr += 64;
          continue;
        }
        if (bitset_->bits_[bit_nr / 64][bit_nr % 64]) {
          break;
        }
        ++bit_nr;
      }
      pos_ = bitset_->low_ + bit_nr;
    }

   public:
    iterator& operator++() {
      if (pos_ < bitset_->high_) {
        ++pos_;
        next();
      }
      return *this;
    }

    bool operator==(iterator other) const {
      return bitset_ == other.bitset_ && pos_ == other.pos_;
    }

    bool operator!=(iterator other) const { return !(*this == other); }

    Symbol operator*() { return Symbol(pos_); }

    friend class SymbolSet;
  };

  iterator begin() const {
    iterator it(this, low_);
    it.next();
    return it;
  }

  iterator end() const { return iterator(this, high_); }

 private:
  friend class iterator;

  /* Ensure that given bit number is in [low_, high_)  */
  void resize(size_t bit_nr) {
    size_t new_low = bit_nr & ~63;
    size_t new_high = (bit_nr + 64) & ~63;
    if (bits_.empty()) {
      high_ = low_ = new_low;
    }
    if (new_low > low_) {
      new_low = low_;
    }
    if (new_high <= high_) {
      new_high = high_;
    }
    if (new_low == low_) {
      bits_.resize((new_high - new_low) / 64);
    } else {
      std::vector<std::bitset<64> > newbits((new_high - new_low) / 64);
      std::copy(bits_.begin(), bits_.end(),
                newbits.begin() + (low_ - new_low) / 64);
      bits_.swap(newbits);
    }
    low_ = new_low;
    high_ = new_high;
  }

  /* Keep only the (aligned) range where at least one bit has been set.
   * E.g., if we only ever set bits 65 and 141, |low_| will be 64, |high_|
   * will be 192, and |bits_| will have 2 elements.
   */
  size_t low_;
  size_t high_;
  std::vector<std::bitset<64> > bits_;
};

class ScopedGlobalVar {
 public:
  ScopedGlobalVar(Symbol name, Var* var);
  ~ScopedGlobalVar();

 private:
  Symbol name_;
  Var* orig_;
};

inline bool operator==(const Symbol& x, const Symbol& y) {
  return x.val() == y.val();
}

inline bool operator<(const Symbol& x, const Symbol& y) {
  return x.val() < y.val();
}

namespace std {
template <>
struct hash<Symbol> {
  size_t operator()(const Symbol& s) const { return s.val(); }
};
}  // namespace std

extern Symbol kEmptySym;
extern Symbol kShellSym;
extern Symbol kAllowRulesSym;
extern Symbol kKatiReadonlySym;

Symbol Intern(std::string_view s);

std::string JoinSymbols(const std::vector<Symbol>& syms, const char* sep);

// Get all symbol names for which filter returns true.
std::vector<std::string_view> GetSymbolNames(
    std::function<bool(Var*)> const& filter);

#endif  // SYMTAB_H_
