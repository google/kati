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

#include <algorithm>
#include <mutex>
#include <vector>

#include "find.h"
#include "flags.h"
#include "log.h"
#include "stringprintf.h"
#include "timeutil.h"

namespace {

std::mutex g_mu;
std::vector<Stats*>* g_stats;

}  // namespace

Stats::Stats(const char* name) : name_(name), elapsed_(0), cnt_(0) {
  std::unique_lock<std::mutex> lock(g_mu);
  if (g_stats == NULL)
    g_stats = new std::vector<Stats*>;
  g_stats->push_back(this);
}

void Stats::DumpTop() const {
  std::unique_lock<std::mutex> lock(mu_);
  if (detailed_.size() > 0) {
    std::vector<std::pair<std::string, StatsDetails>> details(detailed_.begin(),
                                                              detailed_.end());
    sort(details.begin(), details.end(),
         [](const std::pair<std::string, StatsDetails> a,
            const std::pair<std::string, StatsDetails> b) -> bool {
           return a.second.elapsed_ > b.second.elapsed_;
         });

    // Only print the top 10
    details.resize(std::min(details.size(), static_cast<size_t>(10)));

    if (!interesting_.empty()) {
      // No need to print anything out twice
      auto interesting = interesting_;
      for (auto& [n, detail] : details) {
        interesting.erase(n);
      }

      for (auto& name : interesting) {
        auto detail = detailed_.find(name);
        if (detail == detailed_.end()) {
          details.emplace_back(name, StatsDetails());
          continue;
        }
        details.emplace_back(*detail);
      }
    }

    int max_cnt_len = 1;
    for (auto& [name, detail] : details) {
      max_cnt_len = std::max(
          max_cnt_len, static_cast<int>(std::to_string(detail.cnt_).length()));
    }

    for (auto& [name, detail] : details) {
      LOG_STAT(" %6.3f / %*d %s", detail.elapsed_, max_cnt_len, detail.cnt_,
               name.c_str());
    }
  }
}

std::string Stats::String() const {
  std::unique_lock<std::mutex> lock(mu_);
  if (!detailed_.empty())
    return StringPrintf("%s: %f / %d (%d unique)", name_, elapsed_, cnt_,
                        detailed_.size());
  return StringPrintf("%s: %f / %d", name_, elapsed_, cnt_);
}

double Stats::Start() {
  double start = GetTime();
  std::unique_lock<std::mutex> lock(mu_);
  cnt_++;
  return start;
}

double Stats::End(double start, const char* msg) {
  double e = GetTime() - start;
  std::unique_lock<std::mutex> lock(mu_);
  elapsed_ += e;
  if (msg != 0) {
    StatsDetails& details = detailed_[std::string(msg)];
    details.elapsed_ += e;
    details.cnt_++;
  }
  return e;
}

void Stats::MarkInteresting(const std::string& msg) {
  std::unique_lock<std::mutex> lock(mu_);
  interesting_.emplace(msg);
}

ScopedStatsRecorder::ScopedStatsRecorder(Stats* st, const char* msg)
    : st_(st), msg_(msg), start_time_(0) {
  if (!g_flags.enable_stat_logs)
    return;
  start_time_ = st_->Start();
}

ScopedStatsRecorder::~ScopedStatsRecorder() {
  if (!g_flags.enable_stat_logs)
    return;
  double e = st_->End(start_time_, msg_);
  if (msg_ && e > 3.0) {
    LOG_STAT("slow %s (%f): %s", st_->name_, e, msg_);
  }
}

void ReportAllStats() {
  if (!g_stats)
    return;
  for (Stats* st : *g_stats) {
    LOG_STAT("%s", st->String().c_str());
    st->DumpTop();
  }
  delete g_stats;

  LOG_STAT("%u find nodes", FindEmulator::GetNodeCount());
}
