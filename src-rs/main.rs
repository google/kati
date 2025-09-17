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

use std::ffi::{OsStr, OsString};
use std::io::{Write, stdout};
use std::os::unix::ffi::OsStrExt;
use std::sync::Arc;

use anyhow::{Result, bail};
use bytes::{BufMut, Bytes, BytesMut};
use parking_lot::Mutex;

#[cfg(feature = "gperf")]
use gperftools::{HEAP_PROFILER, PROFILER};

use kati::dep::{NamedDepNode, make_dep};
use kati::fileutil::clear_glob_cache;
use kati::log;
use kati::ninja::generate_ninja;
use kati::regen::needs_regen;
use kati::regen_dump::stamp_dump_main;

use kati::eval::FrameType;
use kati::expr::{Evaluable, Value};
use kati::loc::Loc;
use kati::stmt::Stmt;
use kati::var::{VarOrigin, Variable};

use kati::eval::Evaluator;
use kati::flags::FLAGS;
use kati::symtab::{Symbol, intern, join_symbols};
use kati::timeutil::ScopedTimeReporter;

#[cfg(all(not(feature = "gperf"), target_os = "linux"))]
use tikv_jemallocator::Jemalloc;

// Use jemalloc for better performance, but gperftools will use tcmalloc for
// heap debugging.
#[cfg(all(not(feature = "gperf"), target_os = "linux"))]
#[global_allocator]
static GLOBAL: Jemalloc = Jemalloc;

fn read_bootstrap_makefile(targets: &[Symbol]) -> Result<Arc<Mutex<Vec<Stmt>>>> {
    let mut bootstrap = BytesMut::new();
    bootstrap.put_slice(b"CC?=cc\n");
    if cfg!(target_os = "macos") {
        bootstrap.put_slice(b"CXX?=c++\n");
    } else {
        bootstrap.put_slice(b"CXX?=g++\n");
    }
    bootstrap.put_slice(b"AR?=ar\n");
    // Pretend to be GNU make 4.2.1, for compatibility.
    bootstrap.put_slice(b"MAKE_VERSION?=4.2.1\n");
    bootstrap.put_slice(b"KATI?=ckati\n");
    // Overwrite $SHELL environment variable.
    bootstrap.put_slice(b"SHELL=/bin/sh\n");
    // TODO: Add more builtin vars.

    if !FLAGS.no_builtin_rules {
        // http://www.gnu.org/software/make/manual/make.html#Catalogue-of-Rules
        // The document above is actually not correct. See default.c:
        // http://git.savannah.gnu.org/cgit/make.git/tree/default.c?id=4.1
        bootstrap.put_slice(b".c.o:\n");
        bootstrap.put_slice(b"\t$(CC) $(CFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n");
        bootstrap.put_slice(b".cc.o:\n");
        bootstrap.put_slice(b"\t$(CXX) $(CXXFLAGS) $(CPPFLAGS) $(TARGET_ARCH) -c -o $@ $<\n");
        // TODO: Add more builtin rules.
    }
    if FLAGS.generate_ninja {
        bootstrap.put_slice(format!("MAKE?=make -j{}\n", FLAGS.num_jobs.max(1)).as_bytes());
    } else {
        bootstrap.put_slice(b"MAKE?=");
        bootstrap.put_slice(FLAGS.subkati_args.join(OsStr::new(" ")).as_bytes());
        bootstrap.put_u8(b'\n');
    }
    bootstrap.put_slice(b"MAKECMDGOALS?=");
    bootstrap.put(join_symbols(targets, b" "));
    bootstrap.put_u8(b'\n');

    bootstrap.put_slice(b"CURDIR:=");
    bootstrap.put_slice(std::env::current_dir()?.as_os_str().as_bytes());
    bootstrap.put_u8(b'\n');

    kati::parser::parse_buf(
        &bootstrap.freeze(),
        Loc {
            filename: intern("*bootstrap*"),
            line: 0,
        },
    )
}

fn run(targets: &[Symbol], cl_vars: &Vec<Bytes>, orig_args: OsString) -> Result<i32> {
    let start_time = std::time::SystemTime::now();

    if FLAGS.generate_ninja && (FLAGS.regen || FLAGS.dump_kati_stamp) {
        let _tr = ScopedTimeReporter::new("regen_check_time");
        if !needs_regen(start_time, &orig_args) {
            eprintln!("No need to regenerate ninja file");
            return Ok(0);
        }
        if FLAGS.dump_kati_stamp {
            println!("Need to regenerate ninja file");
            return Ok(0);
        }
        clear_glob_cache();
    }

    let mut ev = Evaluator::new();
    ev.start()?;
    let mut makefile_list = BytesMut::new();
    makefile_list.put_u8(b' ');
    makefile_list.put_slice(FLAGS.makefile.lock().clone().unwrap().as_bytes());
    intern("MAKEFILE_LIST").set_global_var(
        Variable::with_simple_string(
            makefile_list.freeze(),
            VarOrigin::File,
            Some(ev.current_frame()),
            ev.loc.clone(),
        ),
        false,
        None,
    )?;
    for (k, v) in std::env::vars_os() {
        let v = Bytes::from(v.as_bytes().to_vec());
        let val = Arc::new(Value::Literal(None, v.clone()));
        intern(k.as_bytes().to_vec()).set_global_var(
            Variable::new_recursive(
                val,
                VarOrigin::Environment,
                Some(ev.current_frame()),
                None,
                v,
            ),
            false,
            None,
        )?;
    }

    let bootstrap_asts = read_bootstrap_makefile(targets)?;

    {
        let _frame = ev.enter(
            FrameType::Phase,
            Bytes::from_static(b"*bootstrap*"),
            Loc::default(),
        );
        ev.in_bootstrap();
        for stmt in bootstrap_asts.lock().iter() {
            log!("{stmt:?}");
            stmt.eval(&mut ev)?;
        }
    }

    {
        let _frame = ev.enter(
            FrameType::Phase,
            Bytes::from_static(b"*command line*"),
            Loc::default(),
        );
        ev.in_command_line();
        for l in cl_vars {
            let asts = kati::parser::parse_buf(
                l,
                Loc {
                    filename: intern("*bootstrap*"),
                    line: 0,
                },
            )?;
            let asts = asts.lock();
            assert!(asts.len() == 1);
            asts[0].eval(&mut ev)?;
        }
    }
    ev.in_toplevel_makefile();

    {
        let _eval_frame = ev.enter(
            FrameType::Phase,
            Bytes::from_static(b"*parse*"),
            Loc::default(),
        );
        let _tr = ScopedTimeReporter::new("eval time");

        let makefile = FLAGS.makefile.lock().clone().unwrap();
        let _file_frame = ev.enter(
            FrameType::Parse,
            Bytes::from(makefile.as_bytes().to_vec()),
            Loc::default(),
        );
        let Some(mk) = kati::file_cache::get_makefile(&makefile)? else {
            bail!("makefile not found")
        };
        let stmts = mk.stmts.lock();
        for stmt in stmts.iter() {
            log!("{stmt:?}");
            stmt.eval(&mut ev)?;
        }
    }

    if let Some(filename) = &FLAGS.dump_include_graph {
        ev.dump_include_json(filename)?;
    }

    let nodes: Vec<NamedDepNode>;
    {
        let _frame = ev.enter(
            FrameType::Phase,
            Bytes::from_static(b"*dependency analysis*"),
            Loc::default(),
        );
        let _tr = ScopedTimeReporter::new("make dep time");
        nodes = make_dep(&mut ev, targets.to_owned())?;
    }

    if FLAGS.is_syntax_check_only {
        return Ok(0);
    }

    if FLAGS.generate_ninja {
        let _frame = ev.enter(
            FrameType::Phase,
            Bytes::from_static(b"*ninja generation*"),
            Loc::default(),
        );
        let _tr = ScopedTimeReporter::new("generate ninja time");
        generate_ninja(&nodes, &mut ev, orig_args.as_bytes(), start_time)?;
        ev.finish()?;
        return Ok(0);
    }

    for (name, export) in ev.exports.clone() {
        if export {
            let value = if let Some(v) = ev.lookup_var(name)? {
                v.read().eval_to_buf(&mut ev)?
            } else {
                Bytes::new()
            };
            log!("setenv({name}, {})", String::from_utf8_lossy(&value));
            // SAFETY: we're single threaded here. If that changes, we could pass the
            // expected environment to the children explicitly.
            unsafe {
                std::env::set_var(
                    OsStr::from_bytes(&name.as_bytes()),
                    OsStr::from_bytes(&value),
                );
            }
        } else {
            log!("unsetenv({name})");
            // SAFETY: we're single threaded here. If that changes, we could pass the
            // expected environment to the children explicitly.
            unsafe {
                std::env::remove_var(OsStr::from_bytes(&name.as_bytes()));
            }
        }
    }

    {
        let _frame = ev.enter(
            FrameType::Phase,
            Bytes::from_static(b"*execution*"),
            Loc::default(),
        );
        let _tr = ScopedTimeReporter::new("exec time");
        kati::exec::exec(nodes, &mut ev)?;
    }

    ev.finish()?;

    Ok(0)
}

fn find_first_makefile() {
    let mut makefile = FLAGS.makefile.lock();
    if makefile.is_some() {
        return;
    }
    if std::fs::exists("GNUMakefile").unwrap_or(false) {
        *makefile = Some(OsString::from("GNUMakefile"));
    } else if !cfg!(target_os = "macos") && std::fs::exists("makefile").unwrap_or(false) {
        *makefile = Some(OsString::from("makefile"));
    } else if std::fs::exists("Makefile").unwrap_or(false) {
        *makefile = Some(OsString::from("Makefile"));
    }
}

fn handle_realpath(args: Vec<String>) {
    for arg in args {
        if let Ok(path) = std::fs::canonicalize(&arg) {
            let _ = stdout().write_all(path.as_os_str().as_bytes());
            println!();
        }
    }
}

fn main() {
    env_logger::builder()
        .filter_level(log::LevelFilter::Warn)
        .format(|buf, record| {
            if let (Some(file), Some(line)) = (record.file(), record.line()) {
                writeln!(buf, "*kati*: {file}:{line}: {}", record.args())
            } else {
                writeln!(buf, "*kati*: {}", record.args())
            }
        })
        .parse_env("KATI_LOG")
        .init();

    if std::env::args().len() >= 2 {
        let arg = std::env::args().nth(1).unwrap();
        if arg == "--realpath" {
            handle_realpath(std::env::args().skip(2).collect());
            return;
        } else if arg == "--dump_stamp_tool" {
            // Unfortunately, this can easily be confused with --dump_kati_stamp,
            // which prints debug info about the stamp while executing a normal kati
            // run. This tool flag only dumps information, and doesn't run the rest of
            // kati.
            if let Err(err) = stamp_dump_main() {
                eprintln!("{err}");
                std::process::exit(1);
            }
            return;
        }
    }

    #[cfg(feature = "gperf")]
    {
        if let Some(path) = &FLAGS.cpu_profile_path {
            PROFILER
                .lock()
                .unwrap()
                .start(std::ffi::CString::new(path.as_bytes()).unwrap())
                .unwrap();
        }
        if let Some(path) = &FLAGS.memory_profile_path {
            HEAP_PROFILER
                .lock()
                .unwrap()
                .start(std::ffi::CString::new(path.as_bytes()).unwrap())
                .unwrap();
        }
    }

    if let Some(working_dir) = &FLAGS.working_dir
        && let Err(e) = std::env::set_current_dir(working_dir)
    {
        eprintln!("*** {}: {}", working_dir.to_string_lossy(), e);
        std::process::exit(1);
    }
    let orig_args = std::env::args_os()
        .collect::<Vec<OsString>>()
        .join(OsStr::new(" "));
    find_first_makefile();
    if FLAGS.makefile.lock().is_none() {
        eprintln!("*** No targets specified and no makefile found.");
        std::process::exit(1);
    }
    let ret = match run(&FLAGS.targets, &FLAGS.cl_vars, orig_args) {
        Ok(ret) => ret,
        Err(err) => {
            for cause in err.chain() {
                eprintln!("{cause}");
            }
            1
        }
    };
    #[cfg(feature = "gperf")]
    {
        if FLAGS.cpu_profile_path.is_some() {
            PROFILER.lock().unwrap().stop().unwrap();
        }
        if FLAGS.memory_profile_path.is_some() {
            HEAP_PROFILER.lock().unwrap().stop().unwrap();
        }
    }
    kati::stats::report_all_stats();
    std::process::exit(ret);
}
