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

#include <mutex>
#include <vector>

#include "flags.h"
#include "log.h"
#include "stringprintf.h"
#include "thread_local.h"
#include "timeutil.h"

namespace {

mutex g_mu;
vector<Stats*>* g_stats;
DEFINE_THREAD_LOCAL(double, g_start_time);

}  // namespace

Stats::Stats(const char* name)
    : name_(name), elapsed_(0), cnt_(0) {
  unique_lock<mutex> lock(g_mu);
  if (g_stats == NULL)
    g_stats = new vector<Stats*>;
  g_stats->push_back(this);
}

string Stats::String() const {
  unique_lock<mutex> lock(mu_);
  return StringPrintf("%s: %f / %d", name_, elapsed_, cnt_);
}

void Stats::Start() {
  CHECK(!TLS_REF(g_start_time));
  TLS_REF(g_start_time) = GetTime();
  unique_lock<mutex> lock(mu_);
  cnt_++;
}

double Stats::End() {
  CHECK(TLS_REF(g_start_time));
  double e = GetTime() - TLS_REF(g_start_time);
  TLS_REF(g_start_time) = 0;
  unique_lock<mutex> lock(mu_);
  elapsed_ += e;
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
