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

use crate::{
    collect_stats, collect_stats_with_slow_report,
    fileutil::{glob, run_command},
    flags::FLAGS,
    func::CommandOp,
    io::{dump_systemtime, load_int, load_string, load_systemtime, load_usize, load_vec_string},
    ninja::{get_ninja_filename, get_ninja_shell_script_filename, get_ninja_stamp_filename},
    strutil::format_for_command_substitution,
};
use anyhow::Result;
use bytes::Bytes;
use parking_lot::Mutex;
use std::{
    ffi::{OsStr, OsString},
    fs::OpenOptions,
    io::{BufReader, Write},
    os::unix::ffi::{OsStrExt, OsStringExt},
    time::SystemTime,
};

fn should_ignore_dirty(s: &[u8]) -> bool {
    let Some(ignore) = &FLAGS.ignore_dirty_pattern else {
        return false;
    };
    ignore.matches(s)
        && !FLAGS
            .no_ignore_dirty_pattern
            .as_ref()
            .map(|p| p.matches(s))
            .unwrap_or(false)
}

struct GlobResult {
    pat: Bytes,
    result: Vec<Bytes>,
}

struct ShellResult {
    op: CommandOp,
    shell: OsString,
    shellflag: OsString,
    cmd: OsString,
    result: Vec<u8>,
    missing_dirs: Vec<OsString>,
    files: Vec<OsString>,
    read_dirs: Vec<OsString>,
}

struct StampChecker {
    gen_time: Option<SystemTime>,
    globs: Vec<GlobResult>,
    commands: Vec<ShellResult>,
    needs_regen: bool,
}

macro_rules! load {
    ($v:expr) => {
        match $v {
            Some(s) => s,
            None => {
                eprintln!("incomplete kati_stamp, regenerating...");
                return true;
            }
        }
    };
}

impl StampChecker {
    fn new() -> Self {
        Self {
            gen_time: None,
            globs: Vec::new(),
            commands: Vec::new(),
            needs_regen: false,
        }
    }

    fn needs_regen(&mut self, start_time: SystemTime, orig_args: &OsStr) -> bool {
        if Self::is_missing_outputs() {
            return true;
        }

        if self.check_step1(orig_args) {
            return true;
        }

        if self.check_step2().unwrap_or(true) {
            return true;
        }

        if !self.needs_regen {
            let mut opts = OpenOptions::new();
            let Ok(mut fp) = opts.write(true).open(get_ninja_stamp_filename()) else {
                return true;
            };
            dump_systemtime(&mut fp, &start_time).unwrap();
        }
        self.needs_regen
    }

    fn is_missing_outputs() -> bool {
        let f = get_ninja_filename();
        if !std::fs::exists(&f).is_ok_and(|b| b) {
            eprintln!("{} is missing, regenerating...", f.to_string_lossy());
            return true;
        }
        let f = get_ninja_shell_script_filename();
        if !std::fs::exists(&f).is_ok_and(|b| b) {
            eprintln!("{} is missing, regenerating...", f.to_string_lossy());
            return true;
        }
        false
    }

    fn check_step1(&mut self, orig_args: &OsStr) -> bool {
        let stamp_filename = get_ninja_stamp_filename();
        let fp = match std::fs::File::open(&stamp_filename) {
            Ok(fp) => fp,
            Err(err) => {
                if FLAGS.regen_debug {
                    println!("{stamp_filename:?}: {err}")
                }
                return true;
            }
        };
        let mut fp = BufReader::new(fp);
        let fp = &mut fp;

        let gen_time = load!(load_systemtime(fp));
        self.gen_time = Some(gen_time);
        if FLAGS.regen_debug {
            println!("Generated time: {:?}", self.gen_time);
        }

        let files = load!(load_vec_string(fp));
        for s in files {
            let ts = std::fs::metadata(&s).and_then(|m| m.modified());
            if ts.as_ref().is_ok_and(|ts| gen_time >= *ts) {
                if FLAGS.dump_kati_stamp {
                    println!("file {s:?}: clean ({:?})", ts.unwrap())
                }
            } else {
                if FLAGS.regen_ignoring_kati_binary && s == std::env::current_exe().unwrap() {
                    eprintln!("{s:?} was modified, ignored.");
                    continue;
                }
                if should_ignore_dirty(s.as_bytes()) {
                    if FLAGS.regen_debug {
                        println!("file {s:?}: ignored ({:?})", ts.unwrap());
                    }
                    continue;
                }
                if FLAGS.dump_kati_stamp {
                    println!("file {s:?}: dirty ({:?})", ts.unwrap());
                } else {
                    eprintln!("{} was modified, regenerating...", s.to_string_lossy());
                }
                return true;
            }
        }

        let undefineds = load!(load_vec_string(fp));
        for s in undefineds {
            if let Ok(v) = std::env::var(&s) {
                if FLAGS.dump_kati_stamp {
                    println!("env {s:?}: dirty (unset => {v:?})");
                } else {
                    eprintln!(
                        "Environment variable {} was set, regenerating...",
                        s.to_string_lossy()
                    );
                }
                return true;
            } else if FLAGS.dump_kati_stamp {
                println!("env {s:?}: clean (unset)");
            }
        }

        let num_envs = load!(load_usize(fp));
        for _ in 0..num_envs {
            let s = load!(load_string(fp));
            let val = std::env::var_os(&s).unwrap_or_default();
            let s2 = load!(load_string(fp));
            if val != s2 {
                if FLAGS.dump_kati_stamp {
                    println!("env {s:?}: dirty ({s2:?} => {val:?})")
                } else {
                    eprintln!(
                        "Environment variable {} was modified ({:?} => {:?}), regenerating...",
                        s.to_string_lossy(),
                        s2.to_string_lossy(),
                        val.to_string_lossy(),
                    );
                }
                return true;
            } else if FLAGS.dump_kati_stamp {
                println!("env {s:?}: clean ({val:?})")
            }
        }

        let num_globs = load!(load_usize(fp));
        for _ in 0..num_globs {
            let pat = load!(load_string(fp));
            let result = load!(load_vec_string(fp));
            self.globs.push(GlobResult {
                pat: Bytes::from(pat.into_vec()),
                result: result
                    .into_iter()
                    .map(|r| Bytes::from(r.into_vec()))
                    .collect(),
            })
        }

        let num_crs = load!(load_usize(fp));
        for _ in 0..num_crs {
            let op = load!(load_int(fp).and_then(CommandOp::from_int));
            let shell = load!(load_string(fp));
            let shellflag = load!(load_string(fp));
            let cmd = load!(load_string(fp));
            let result = load!(load_string(fp));
            let mut sr = ShellResult {
                op,
                shell,
                shellflag,
                cmd,
                result: result.into_vec(),
                missing_dirs: Vec::new(),
                files: Vec::new(),
                read_dirs: Vec::new(),
            };

            // Ignore debug info
            load!(load_string(fp));
            load!(load_int(fp));

            if op == CommandOp::Find {
                sr.missing_dirs = load!(load_vec_string(fp));
                sr.files = load!(load_vec_string(fp));
                sr.read_dirs = load!(load_vec_string(fp));
            }
            self.commands.push(sr);
        }

        let s = load!(load_string(fp));
        if orig_args != s {
            eprintln!("arguments changed, regenerating...");
            return true;
        }

        self.needs_regen
    }

    fn check_glob_result(gr: &GlobResult, err: &mut String) -> bool {
        collect_stats!("glob time (regen)");
        let files = glob(gr.pat.clone());
        let needs_regen = if let Ok(files) = files.as_ref() {
            files != &gr.result
        } else {
            true
        };
        if needs_regen {
            if should_ignore_dirty(&gr.pat) {
                if FLAGS.dump_kati_stamp {
                    println!("wildcard {:?}: ignored", gr.pat);
                }
                return false;
            }
            if FLAGS.dump_kati_stamp {
                println!("wildcard {:?}: dirty", gr.pat);
            } else {
                *err = format!(
                    "wildcard({}) was changed, regenerating...",
                    String::from_utf8_lossy(&gr.pat)
                );
            }
        } else if FLAGS.dump_kati_stamp {
            println!("wildcard {:?}: clean", gr.pat);
        }
        needs_regen
    }

    fn should_run_command(sr: &ShellResult, gen_time: SystemTime) -> bool {
        if sr.op != CommandOp::Find {
            return true;
        }

        collect_stats!("stat time (regen)");
        for dir in &sr.missing_dirs {
            if std::fs::exists(dir).unwrap_or(false) {
                return true;
            }
        }
        for file in &sr.files {
            if !std::fs::exists(file).unwrap_or(false) {
                return true;
            }
        }
        for dir in &sr.read_dirs {
            // We assume we rarely do a significant change for the top
            // directory which affects the results of find command.
            if dir.is_empty() || dir == "." || should_ignore_dirty(dir.as_bytes()) {
                continue;
            }

            let Ok(md) = std::fs::symlink_metadata(dir) else {
                return true;
            };
            let Ok(ts) = md.modified() else {
                return true;
            };
            if gen_time < ts {
                return true;
            }
            if md.is_symlink() {
                let Ok(ts) = std::fs::metadata(dir).and_then(|md| md.modified()) else {
                    return true;
                };
                if gen_time < ts {
                    return true;
                }
            }
        }
        false
    }

    fn check_shell_result(
        sr: &ShellResult,
        gen_time: SystemTime,
        err: &mut String,
    ) -> Result<bool> {
        if sr.op == CommandOp::ReadMissing {
            if std::fs::exists(&sr.cmd).unwrap_or(false) {
                if FLAGS.dump_kati_stamp {
                    println!("file {:?}: dirty", sr.cmd);
                } else {
                    *err = format!(
                        "$(file <{}) was changed, regenerating...",
                        sr.cmd.to_string_lossy()
                    );
                }
                return Ok(true);
            }
            if FLAGS.dump_kati_stamp {
                println!("file {:?}: clean", sr.cmd);
            }
            return Ok(false);
        }

        if sr.op == CommandOp::Read {
            let ts = std::fs::metadata(&sr.cmd).and_then(|md| md.modified());
            if ts.is_ok_and(|ts| gen_time < ts) {
                if FLAGS.dump_kati_stamp {
                    println!("file {:?}: dirty", sr.cmd);
                } else {
                    *err = format!(
                        "$(file <{}) was changed, regenerating...",
                        sr.cmd.to_string_lossy()
                    );
                }
                return Ok(true);
            }
            if FLAGS.dump_kati_stamp {
                println!("file {:?}: clean", sr.cmd);
            }
            return Ok(false);
        }

        if sr.op == CommandOp::Write || sr.op == CommandOp::Append {
            let mut opts = OpenOptions::new();
            opts.write(true)
                .create(true)
                .append(sr.op == CommandOp::Append);
            let mut f = opts.open(&sr.cmd)?;
            f.write_all(&sr.result)?;

            if FLAGS.dump_kati_stamp {
                println!("file {:?}: clean (write)", sr.cmd);
            }
            return Ok(false);
        }

        if !Self::should_run_command(sr, gen_time) {
            if FLAGS.regen_debug {
                println!("shell {:?}: clean (no rerun)", sr.cmd);
            }
            return Ok(false);
        }

        let cmd = Bytes::from(sr.cmd.clone().into_vec());
        if let Some(fc) = crate::find::parse(&cmd)?
            && fc.chdir.is_some_and(|d| should_ignore_dirty(&d))
        {
            if FLAGS.dump_kati_stamp {
                println!("shell {:?}: ignored", sr.cmd);
            }
            return Ok(false);
        }

        collect_stats_with_slow_report!("shell time (regen)", &sr.cmd);
        let (_status, output) = run_command(
            sr.shell.as_bytes(),
            sr.shellflag.as_bytes(),
            &cmd,
            crate::fileutil::RedirectStderr::DevNull,
        )?;
        let output = format_for_command_substitution(output);
        if sr.result != output {
            if FLAGS.dump_kati_stamp {
                println!("shell {:?}: dirty", sr.cmd);
            } else {
                *err = format!(
                    "$(shell {}) was changed, regenerating...",
                    sr.cmd.to_string_lossy()
                );
            }
            return Ok(true);
        } else if FLAGS.regen_debug {
            println!("shell {:?}: clean (rerun)", sr.cmd);
        }
        Ok(false)
    }

    fn check_step2(&mut self) -> Result<bool> {
        let needs_regen: Mutex<Result<bool>> = Mutex::new(Ok(false));

        std::thread::scope(|s| {
            s.spawn(|| {
                let mut err = String::new();
                for gr in &self.globs {
                    if Self::check_glob_result(gr, &mut err) {
                        let mut needs_regen = needs_regen.lock();
                        if let Ok(false) = *needs_regen {
                            *needs_regen = Ok(true);
                            eprintln!("{err}");
                        }
                        break;
                    }
                }
            });
            s.spawn(|| {
                let mut err = String::new();
                for sr in &self.commands {
                    match Self::check_shell_result(sr, self.gen_time.unwrap(), &mut err) {
                        Ok(true) => {
                            let mut needs_regen = needs_regen.lock();
                            if let Ok(false) = *needs_regen {
                                *needs_regen = Ok(true);
                                eprintln!("{err}");
                            }
                            break;
                        }
                        Ok(false) => {}
                        Err(e) => {
                            let mut needs_regen = needs_regen.lock();
                            if let Ok(false) = *needs_regen {
                                *needs_regen = Err(e);
                            }
                            break;
                        }
                    }
                }
            });
        });

        needs_regen.into_inner()
    }
}

pub fn needs_regen(start_time: SystemTime, orig_args: &OsStr) -> bool {
    StampChecker::new().needs_regen(start_time, orig_args)
}
