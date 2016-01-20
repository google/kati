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

#include "flags.h"
#include "log.h"
#include "stringprintf.h"
#include "timeutil.h"

namespace {

vector<Stats*>* g_stats;

}  // namespace

Stats::Stats(const char* name)
    : name_(name), start_time_(0), elapsed_(0), cnt_(0) {
  if (g_stats == NULL)
    g_stats = new vector<Stats*>;
  g_stats->push_back(this);
}

string Stats::String() const {
  return StringPrintf("%s: %f / %d", name_, elapsed_, cnt_);
}

void Stats::Start() {
  CHECK(!start_time_);
  cnt_++;
  start_time_ = GetTime();
}

double Stats::End() {
  CHECK(start_time_);
  double e = GetTime() - start_time_;
  elapsed_ += e;
  start_time_ = 0;
  return e;
}

ScopedStatsRecorder::ScopedStatsRecorder(Stats* st, const char* msg)
    : st_(st), msg_(msg) {
  if (!g_flags.enable_stat_logs)
    return;
  st_->Start();
}

ScopedStatsRecorder::~ScopedStatsRecorder() {
  if (!g_flags.enable_stat_logs)
    return;
  double e = st_->End();
  if (msg_ && e > 3.0) {
    LOG_STAT("slow %s (%f): %s", st_->name_, e, msg_);
  }
}

void ReportAllStats() {
  if (!g_stats)
    return;
  for (Stats* st : *g_stats) {
    LOG_STAT("%s", st->String().c_str());
  }
  delete g_stats;
}
