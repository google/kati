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

#include "stats.h"

#include <vector>

#include "log.h"
#include "stringprintf.h"
#include "time.h"

namespace {

vector<Stats*>* g_stats;

}  // namespace

Stats::Stats(const char* name)
    : name_(name), start_time_(0), elapsed_(0) {
  if (g_stats == NULL)
    g_stats = new vector<Stats*>;
  g_stats->push_back(this);
}

string Stats::String() const {
  return StringPrintf("%s: %f", name_, elapsed_);
}

void Stats::Start() {
  start_time_ = GetTime();
}

void Stats::End() {
  elapsed_ += GetTime() - start_time_;
}

ScopedStatsRecorder::ScopedStatsRecorder(Stats* st)
    : st_(st) {
  st_->Start();
}

ScopedStatsRecorder::~ScopedStatsRecorder() {
  st_->End();
}

void ReportAllStats() {
  if (!g_stats)
    return;
  for (Stats* st : *g_stats) {
    LOG_STAT("%s", st->String().c_str());
  }
  delete g_stats;
}
