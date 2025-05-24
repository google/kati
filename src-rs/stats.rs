/*
Copyright 2025 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//! `kati::stats` implements stats collection and reporting about regions of
//! execution.

use crate::{flags::FLAGS, symtab::symbol_count};
use parking_lot::Mutex;
use std::{
    collections::{HashMap, HashSet},
    ffi::{OsStr, OsString},
    fmt::Display,
    sync::Arc,
    time::{Duration, Instant},
};

static ALL_STATS: Mutex<Vec<Arc<Stats>>> = Mutex::new(Vec::new());

#[derive(Default, Clone)]
struct StatsDetails {
    count: i64,
    elapsed: Duration,
}

/// `Stats` represents a single collection site.
pub struct Stats {
    name: String,
    count: Mutex<i64>,
    elapsed: Mutex<Duration>,
    detailed: Mutex<HashMap<OsString, StatsDetails>>,
    interesting: Mutex<HashSet<OsString>>,
}

impl Stats {
    /// Create a new `Stats` instance. Normally you would use [`collect_stats!`]
    /// or [`collect_stats_with_slow_report!`] to call this.
    #[doc(hidden)]
    pub fn new(name: &str) -> Arc<Self> {
        let stats = Arc::new(Self {
            name: name.to_string(),
            elapsed: Mutex::new(Duration::new(0, 0)),
            count: Mutex::new(0),
            detailed: Mutex::new(HashMap::new()),
            interesting: Mutex::new(HashSet::new()),
        });
        let mut all_stats = ALL_STATS.lock();
        all_stats.push(stats.clone());
        stats
    }

    fn dump_top(&self) {
        let all_details = self.detailed.lock();
        if all_details.is_empty() {
            return;
        }

        let mut detailed = all_details
            .iter()
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect::<Vec<_>>();
        detailed.sort_by(|a, b| b.1.elapsed.cmp(&a.1.elapsed));
        // Only print the top 10
        detailed.truncate(10);

        let mut interesting = self.interesting.lock().clone();
        if !interesting.is_empty() {
            // No need to print anything out twice
            for (name, _) in detailed.iter() {
                interesting.remove(name);
            }

            for name in interesting {
                if let Some(details) = all_details.get(&name) {
                    detailed.push((name, details.clone()));
                } else {
                    detailed.push((name, StatsDetails::default()))
                }
            }
        }

        let max_cnt_len = detailed
            .iter()
            .map(|(_, v)| format!("{}", v.count).len())
            .max()
            .unwrap_or(1);
        for (name, details) in detailed {
            eprintln!(
                "*kati*: {:>6.3} / {:>max_cnt_len$} {}",
                details.elapsed.as_secs_f64(),
                details.count,
                name.to_string_lossy()
            );
        }
    }

    /// The implementation behind [`collect_stats!`]
    #[doc(hidden)]
    #[must_use]
    pub fn start_scope(self: &Arc<Self>) -> impl Drop {
        ScopedStatsRecorder::new(self)
    }

    /// The implementation behind [`collect_stats_with_slow_report!`]
    #[doc(hidden)]
    #[must_use]
    pub fn start_scope_with_slow_report(self: &Arc<Self>, msg: &OsStr) -> impl Drop {
        ScopedStatsRecorderWithSlowReport::new(self, msg)
    }

    fn start(&self) -> Instant {
        let start = std::time::Instant::now();
        *self.count.lock() += 1;
        start
    }

    fn end(&self, start: Instant) -> Duration {
        let elapsed = start.elapsed();
        *self.elapsed.lock() += elapsed;
        elapsed
    }

    fn end_with_msg(&self, start: Instant, msg: &OsStr) -> Duration {
        let elapsed = start.elapsed();
        *self.elapsed.lock() += elapsed;
        let mut detailed = self.detailed.lock();
        if let Some(details) = detailed.get_mut(msg) {
            details.count += 1;
            details.elapsed += elapsed;
        } else {
            detailed.insert(msg.to_owned(), StatsDetails { count: 1, elapsed });
        }
        elapsed
    }

    /// Mark the specific execution as interesting. It will be logged even if
    /// it isn't in the top 10 executions.
    pub fn mark_interesting(&self, name: OsString) {
        self.interesting.lock().insert(name);
    }
}

impl Display for Stats {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let detailed = self.detailed.lock();
        if !detailed.is_empty() {
            return write!(
                f,
                "{}: {} / {} ({} unique)",
                self.name,
                self.elapsed.lock().as_secs_f64(),
                *self.count.lock(),
                detailed.len()
            );
        }
        write!(
            f,
            "{}: {} / {}",
            self.name,
            self.elapsed.lock().as_secs_f64(),
            *self.count.lock()
        )
    }
}

struct ScopedStatsRecorder<'a> {
    st: &'a Stats,
    start: Instant,
}

impl<'a> ScopedStatsRecorder<'a> {
    fn new(st: &'a Stats) -> Self {
        let start = st.start();
        Self { st, start }
    }
}

impl Drop for ScopedStatsRecorder<'_> {
    fn drop(&mut self) {
        self.st.end(self.start);
    }
}

/// Define and collect statistics about this block of code.
///
/// We'll record both the count and duration of these blocks, and report them
/// when [`report_all_stats`] is called.
#[macro_export]
macro_rules! collect_stats {
    ($name:literal) => {
        static STATS: std::sync::LazyLock<std::sync::Arc<$crate::stats::Stats>> =
            std::sync::LazyLock::new(|| $crate::stats::Stats::new($name));
        let _ssr = if $crate::flags::FLAGS.enable_stat_logs {
            Some(STATS.start_scope())
        } else {
            None
        };
    };
}

struct ScopedStatsRecorderWithSlowReport<'a, 'b> {
    st: &'a Stats,
    msg: &'b OsStr,
    start: Instant,
}

impl<'a, 'b> ScopedStatsRecorderWithSlowReport<'a, 'b> {
    fn new(st: &'a Stats, msg: &'b OsStr) -> Self {
        let start = st.start();
        Self { st, msg, start }
    }
}

impl Drop for ScopedStatsRecorderWithSlowReport<'_, '_> {
    fn drop(&mut self) {
        let dur = self.st.end_with_msg(self.start, self.msg);
        if dur > Duration::from_secs(3) {
            eprintln!(
                "*kati*: slow {} ({}): {}",
                self.st.name,
                dur.as_secs_f64(),
                self.msg.to_string_lossy()
            )
        }
    }
}

/// Define and collect statistics about this block of code, with specific
/// executions identified by the `msg`.
///
/// We'll record both the count and duration of these blocks, and report them
/// when [`report_all_stats`] is called. In addition, the top ten specific
/// instances will have their duration and counts reported as well.
///
/// Any executions over 3 seconds will be logged as they happen.
#[macro_export]
macro_rules! collect_stats_with_slow_report {
    ($name:literal, $msg:expr) => {
        static STATS: std::sync::LazyLock<std::sync::Arc<$crate::stats::Stats>> =
            std::sync::LazyLock::new(|| $crate::stats::Stats::new($name));
        let _ssr = if $crate::flags::FLAGS.enable_stat_logs {
            Some(STATS.start_scope_with_slow_report($msg))
        } else {
            None
        };
    };
}

/// Report all the statistics to stderr, if `--enable_stat_log` is enabled.
pub fn report_all_stats() {
    let all_stats = std::mem::take(&mut *ALL_STATS.lock());
    if FLAGS.enable_stat_logs {
        for stats in all_stats {
            eprintln!("*kati*: {stats}");
            stats.dump_top();
        }
        eprintln!("*kati*: {} symbols", symbol_count());
        eprintln!("*kati*: {} find nodes", crate::find::get_node_count());
    }
}
