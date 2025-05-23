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

// TODO: Add docs
#![allow(missing_docs)]
// These are the lints enabled by default in Android
// #![deny(missing_docs)]
#![deny(warnings)]
#![deny(unsafe_op_in_unsafe_fn)]
#![deny(clippy::undocumented_unsafe_blocks)]

use strutil::trim_prefix_str;

pub mod command;
pub mod dep;
pub mod eval;
pub mod exec;
pub mod expr;
pub mod file;
pub mod file_cache;
pub mod fileutil;
pub mod find;
pub mod flags;
pub mod func;
pub mod io;
pub mod loc;
pub mod ninja;
pub mod parser;
pub mod regen;
pub mod regen_dump;
pub mod rule;
pub mod stats;
pub mod stmt;
pub mod strutil;
pub mod symtab;
pub mod timeutil;
pub mod var;

#[macro_export]
macro_rules! log {
    ($fmt:expr $(, $($arg:tt)*)?) => {
        log::trace!($fmt, $($($arg)*)?)
    };
}

#[macro_export]
macro_rules! log_stat {
    ($fmt:expr $(, $($arg:tt)*)?) => {
        if $crate::flags::FLAGS.enable_stat_logs {
            eprintln!("*kati*: {}", format!($fmt, $($($arg)*)?))
        }
    };
}

#[macro_export]
macro_rules! warn {
    ($fmt:expr $(, $($arg:tt)*)?) => {
        eprintln!($fmt, $($($arg)*)?)
    };
}

#[macro_export]
macro_rules! kati_warn {
    ($fmt:expr $(, $($arg:tt)*)?) => {
        if $crate::flags::FLAGS.enable_kati_warnings {
            eprintln!($fmt, $($($arg)*)?)
        }
    };
}

#[macro_export]
macro_rules! error {
    ($fmt:expr $(, $($arg:tt)*)?) => {
        anyhow::bail!($fmt, $($($arg)*)?)
    };
}

#[macro_export]
macro_rules! warn_loc {
    ($loc:expr, $fmt:expr $(, $($arg:tt)*)?) => {
        $crate::color_warn_log($loc, format!($fmt, $($($arg)*)?))
    };
}

#[macro_export]
macro_rules! kati_warn_loc {
    ($loc:expr, $fmt:expr $(, $($arg:tt)*)?) => {
        if $crate::flags::FLAGS.enable_kati_warnings {
            $crate::color_warn_log($loc, format!($fmt, $($($arg)*)?))
        }
    };
}

#[macro_export]
macro_rules! error_loc {
    ($loc:expr, $fmt:expr $(, $($arg:tt)*)?) => {
        return Err($crate::color_error_log($loc, format!($fmt, $($($arg)*)?)))
    };
}

const BOLD: &str = "\x1b[1m";
const RESET: &str = "\x1b[0m";
const MAGENTA: &str = "\x1b[35m";
const RED: &str = "\x1b[31m";

fn color_error_log(loc: Option<&crate::loc::Loc>, msg: String) -> anyhow::Error {
    let Some(loc) = loc else {
        return anyhow::format_err!("{msg}");
    };

    if crate::flags::FLAGS.color_warnings {
        let filtered = trim_prefix_str(&msg, "*** ");

        anyhow::format_err!("{BOLD}{loc}: {RED}error: {RESET}{BOLD}{filtered}{RESET}")
    } else {
        anyhow::format_err!("{loc}: {msg}")
    }
}

fn color_warn_log(loc: Option<&crate::loc::Loc>, msg: String) {
    let Some(loc) = loc else {
        eprintln!("{msg}");
        return;
    };

    if crate::flags::FLAGS.color_warnings {
        let mut filtered = trim_prefix_str(&msg, "*warning*: ");
        filtered = trim_prefix_str(filtered, "warning: ");

        eprintln!("{BOLD}{loc}: {MAGENTA}warning: {RESET}{BOLD}{filtered}{RESET}")
    } else {
        eprintln!("{loc}: {msg}")
    }
}
