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

#ifndef STATS_H_
#define STATS_H_

#include <mutex>
#include <string>
#include <unordered_map>
#include <unordered_set>

struct StatsDetails {
  int cnt_ = 0;
  double elapsed_ = 0;
};

class Stats {
 public:
  explicit Stats(const char* name);

  void DumpTop() const;
  std::string String() const;

  void MarkInteresting(const std::string& msg);

 private:
  double Start();
  double End(double start, const char* msg);

  friend class ScopedStatsRecorder;

  const char* name_;
  double elapsed_;
  int cnt_;
  mutable std::mutex mu_;
  std::unordered_map<std::string, StatsDetails> detailed_;
  std::unordered_set<std::string> interesting_;
};

class ScopedStatsRecorder {
 public:
  explicit ScopedStatsRecorder(Stats* st, const char* msg = 0);
  ~ScopedStatsRecorder();

 private:
  Stats* st_;
  const char* msg_;
  double start_time_;
};

void ReportAllStats();

#define COLLECT_STATS(name) \
  static Stats stats(name); \
  ScopedStatsRecorder ssr(&stats)

#define COLLECT_STATS_WITH_SLOW_REPORT(name, msg) \
  static Stats stats(name);                       \
  ScopedStatsRecorder ssr(&stats, msg)

#endif  // STATS_H_
