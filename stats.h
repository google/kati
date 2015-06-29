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

#include <string>

using namespace std;

class Stats {
 public:
  explicit Stats(const char* name);

  string String() const;

 private:
  void Start();
  void End();

  friend class ScopedStatsRecorder;

  const char* name_;
  double start_time_;
  double elapsed_;
};

class ScopedStatsRecorder {
 public:
  explicit ScopedStatsRecorder(Stats* st);
  ~ScopedStatsRecorder();

 private:
  Stats* st_;
};

void ReportAllStats();

#define COLLECT_STATS(name)                     \
  static Stats stats(name);                     \
  ScopedStatsRecorder ssr(&stats)

#endif  // STATS_H_
