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

use std::{
    env,
    ffi::{OsStr, OsString},
    os::unix::ffi::{OsStrExt, OsStringExt},
    sync::LazyLock,
    vec::IntoIter,
};

use crate::{
    strutil::{Pattern, word_scanner},
    symtab::intern,
};
use bytes::Bytes;
use parking_lot::Mutex;

pub static FLAGS: LazyLock<Flags> = LazyLock::new(|| {
    if cfg!(test) {
        Flags::default()
    } else {
        Flags::from_args(env::args_os().collect())
    }
});

#[derive(Default)]
pub struct Flags {
    pub detect_android_echo: bool,
    pub detect_depfiles: bool,
    pub dump_kati_stamp: bool,
    pub dump_include_graph: Option<OsString>,
    pub dump_variable_assignment_trace: Option<OsString>,
    pub enable_debug: bool,
    pub enable_kati_warnings: bool,
    pub enable_stat_logs: bool,
    pub gen_all_targets: bool,
    pub generate_ninja: bool,
    pub generate_empty_ninja: bool,
    pub is_dry_run: bool,
    pub is_silent_mode: bool,
    pub is_syntax_check_only: bool,
    pub regen: bool,
    pub regen_debug: bool,
    pub regen_ignoring_kati_binary: bool,
    pub use_find_emulator: bool,
    pub color_warnings: bool,
    pub no_builtin_rules: bool,
    pub no_ninja_prelude: bool,
    pub use_ninja_phony_output: bool,
    pub use_ninja_validations: bool,
    pub werror_find_emulator: bool,
    pub werror_overriding_commands: bool,
    pub warn_implicit_rules: bool,
    pub werror_implicit_rules: bool,
    pub warn_suffix_rules: bool,
    pub werror_suffix_rules: bool,
    pub top_level_phony: bool,
    pub warn_real_to_phony: bool,
    pub werror_real_to_phony: bool,
    pub warn_phony_looks_real: bool,
    pub werror_phony_looks_real: bool,
    pub werror_writable: bool,
    pub warn_real_no_cmds_or_deps: bool,
    pub werror_real_no_cmds_or_deps: bool,
    pub warn_real_no_cmds: bool,
    pub werror_real_no_cmds: bool,
    pub default_pool: OsString,
    pub ignore_dirty_pattern: Option<crate::strutil::Pattern>,
    pub no_ignore_dirty_pattern: Option<crate::strutil::Pattern>,
    pub ignore_optional_include_pattern: Option<crate::strutil::Pattern>,
    pub makefile: Mutex<Option<OsString>>,
    pub ninja_dir: Option<OsString>,
    pub ninja_suffix: OsString,
    pub working_dir: Option<OsString>, // -C <dir>
    pub num_cpus: usize,
    pub num_jobs: usize,
    pub remote_num_jobs: usize,
    pub subkati_args: Vec<OsString>,
    pub targets: Vec<crate::symtab::Symbol>,
    pub cl_vars: Vec<Bytes>,
    pub writable: Vec<OsString>,
    pub traced_variables_pattern: Vec<crate::strutil::Pattern>,

    pub cpu_profile_path: Option<OsString>,
    pub memory_profile_path: Option<OsString>,
}

fn parse_command_line_option_with_arg(
    option: &str,
    arg: &OsStr,
    args: &mut IntoIter<OsString>,
) -> Option<OsString> {
    let arg = arg.as_bytes();
    let arg = arg.strip_prefix(option.as_bytes())?;
    if arg.is_empty() {
        return args.next();
    }
    if let Some(arg) = arg.strip_prefix(b"=") {
        return Some(OsString::from_vec(arg.to_vec()));
    }
    // E.g, -j999
    if option.len() == 2 {
        return Some(OsString::from_vec(arg.to_vec()));
    }
    None
}

impl Flags {
    fn from_args(args: Vec<OsString>) -> Flags {
        let mut iter = args.into_iter();
        let mut flags = Flags::default();
        flags.subkati_args.push(iter.next().unwrap());
        flags.num_jobs = std::thread::available_parallelism().map_or(1, |p| p.get());
        flags.num_cpus = flags.num_jobs;

        if let Some(makeflags) = env::var_os("MAKEFLAGS") {
            for tok in crate::strutil::word_scanner(makeflags.as_bytes()) {
                if !tok.starts_with(b"-") && tok.contains(&b'=') {
                    flags.cl_vars.push(Bytes::from(tok.to_vec()));
                }
            }
        }

        while let Some(arg) = iter.next() {
            let mut should_propagate = true;
            match arg.as_bytes() {
                b"-f" => {
                    *flags.makefile.lock() = iter.next();
                    should_propagate = false;
                }
                b"-c" => flags.is_syntax_check_only = true,
                b"-i" => flags.is_dry_run = true,
                b"-s" => flags.is_silent_mode = true,
                b"-d" => flags.enable_debug = true,
                b"--kati_stats" => flags.enable_stat_logs = true,
                b"--warn" => flags.enable_kati_warnings = true,
                b"--ninja" => flags.generate_ninja = true,
                b"--empty_ninja_file" => flags.generate_empty_ninja = true,
                b"--gen_all_targets" => flags.gen_all_targets = true,
                b"--regen" => {
                    // TODO: Make this default.
                    flags.regen = true
                }
                b"--regen_debug" => flags.regen_debug = true,
                b"--regen_ignoring_kati_binary" => flags.regen_ignoring_kati_binary = true,
                b"--dump_kati_stamp" => {
                    flags.dump_kati_stamp = true;
                    flags.regen_debug = true;
                }
                b"--detect_android_echo" => flags.detect_android_echo = true,
                b"--detect_depfiles" => flags.detect_depfiles = true,
                b"--color_warnings" => flags.color_warnings = true,
                b"--no_builtin_rules" => flags.no_builtin_rules = true,
                b"--no_ninja_prelude" => flags.no_ninja_prelude = true,
                b"--use_ninja_phony_output" => flags.use_ninja_phony_output = true,
                b"--use_ninja_validations" => flags.use_ninja_validations = true,
                b"--werror_find_emulator" => flags.werror_find_emulator = true,
                b"--werror_overriding_commands" => flags.werror_overriding_commands = true,
                b"--warn_implicit_rules" => flags.warn_implicit_rules = true,
                b"--werror_implicit_rules" => flags.werror_implicit_rules = true,
                b"--warn_suffix_rules" => flags.warn_suffix_rules = true,
                b"--werror_suffix_rules" => flags.werror_suffix_rules = true,
                b"--top_level_phony" => flags.top_level_phony = true,
                b"--warn_real_to_phony" => flags.warn_real_to_phony = true,
                b"--werror_real_to_phony" => {
                    flags.warn_real_to_phony = true;
                    flags.werror_real_to_phony = true;
                }
                b"--warn_phony_looks_real" => flags.warn_phony_looks_real = true,
                b"--werror_phony_looks_real" => {
                    flags.warn_phony_looks_real = true;
                    flags.werror_phony_looks_real = true;
                }
                b"--werror_writable" => flags.werror_writable = true,
                b"--warn_real_no_cmds_or_deps" => flags.warn_real_no_cmds_or_deps = true,
                b"--werror_real_no_cmds_or_deps" => {
                    flags.warn_real_no_cmds_or_deps = true;
                    flags.werror_real_no_cmds_or_deps = true;
                }
                b"--warn_real_no_cmds" => flags.warn_real_no_cmds = true,
                b"--werror_real_no_cmds" => {
                    flags.warn_real_no_cmds = true;
                    flags.werror_real_no_cmds = true;
                }
                b"--use_find_emulator" => flags.use_find_emulator = true,
                _ => {
                    if let Some(arg) = parse_command_line_option_with_arg("-C", &arg, &mut iter) {
                        flags.working_dir = Some(arg);
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--dump_include_graph", &arg, &mut iter)
                    {
                        flags.dump_include_graph = Some(arg);
                    } else if let Some(arg) = parse_command_line_option_with_arg(
                        "--dump_variable_assignment_trace",
                        &arg,
                        &mut iter,
                    ) {
                        flags.dump_variable_assignment_trace = Some(arg);
                    } else if let Some(arg) = parse_command_line_option_with_arg(
                        "--variable_assignment_trace_filter",
                        &arg,
                        &mut iter,
                    ) {
                        for pat in word_scanner(arg.as_bytes()) {
                            flags
                                .traced_variables_pattern
                                .push(Pattern::new(Bytes::from(pat.to_vec())));
                        }
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("-j", &arg, &mut iter)
                    {
                        let Some(num_jobs) = arg.to_string_lossy().parse::<usize>().ok() else {
                            panic!("Invalid -j flag: {}", arg.to_string_lossy());
                        };
                        flags.num_jobs = num_jobs;
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--remote_num_jobs", &arg, &mut iter)
                    {
                        let Some(num_jobs) = arg.to_string_lossy().parse::<usize>().ok() else {
                            panic!("Invalid --remote_num_jobs flag: {}", arg.to_string_lossy());
                        };
                        flags.remote_num_jobs = num_jobs;
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--ninja_suffix", &arg, &mut iter)
                    {
                        flags.ninja_suffix = arg;
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--ninja_dir", &arg, &mut iter)
                    {
                        flags.ninja_dir = Some(arg);
                    } else if let Some(arg) = parse_command_line_option_with_arg(
                        "--ignore_optional_include",
                        &arg,
                        &mut iter,
                    ) {
                        flags.ignore_optional_include_pattern =
                            Some(Pattern::new(Bytes::from(arg.as_bytes().to_vec())));
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--ignore_dirty", &arg, &mut iter)
                    {
                        flags.ignore_dirty_pattern =
                            Some(Pattern::new(Bytes::from(arg.as_bytes().to_vec())));
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--no_ignore_dirty", &arg, &mut iter)
                    {
                        flags.no_ignore_dirty_pattern =
                            Some(Pattern::new(Bytes::from(arg.as_bytes().to_vec())));
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--writable", &arg, &mut iter)
                    {
                        flags.writable.push(arg);
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--default_pool", &arg, &mut iter)
                    {
                        flags.default_pool = arg;
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--cpu_profile", &arg, &mut iter)
                    {
                        flags.cpu_profile_path = Some(arg)
                    } else if let Some(arg) =
                        parse_command_line_option_with_arg("--mem_profile", &arg, &mut iter)
                    {
                        flags.memory_profile_path = Some(arg)
                    } else if arg.as_bytes().starts_with(b"-") {
                        panic!("Unknown flag: {}", arg.to_string_lossy());
                    } else if arg.as_bytes().contains(&b'=') {
                        flags.cl_vars.push(Bytes::from(arg.as_bytes().to_vec()));
                    } else {
                        should_propagate = false;
                        let arg = Bytes::from(arg.as_bytes().to_vec());
                        flags.targets.push(intern(arg));
                    }
                }
            }
            if should_propagate {
                flags.subkati_args.push(arg);
            }
        }

        if !flags.traced_variables_pattern.is_empty()
            && flags.dump_variable_assignment_trace.is_none()
        {
            panic!(
                "--variable_assignment_trace_filter is valid only together with --dump_variable_assignment_trace"
            );
        }

        flags
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_flags() {
        let flags = Flags::from_args(
            vec!["test", "-f", "main.mk"]
                .into_iter()
                .map(|s| s.into())
                .collect(),
        );
        assert_eq!(flags.makefile.lock().clone().unwrap(), "main.mk");
    }

    #[test]
    fn test_parse_command_line_option_with_arg() {
        assert_eq!(
            parse_command_line_option_with_arg(
                "--ignore_optional_include",
                &OsString::from("--ignore_optional_include=out/%.P"),
                &mut vec![].into_iter()
            ),
            Some(OsString::from("out/%.P"))
        );
    }
}
