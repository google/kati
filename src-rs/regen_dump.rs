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

use std::{io::BufReader, os::unix::ffi::OsStrExt};

use anyhow::{Context, Result};

use crate::{
    func::CommandOp,
    io::{load_int, load_string, load_systemtime, load_usize, load_vec_string},
};

// This command will dump the contents of a kati stamp file into a more portable
// format for use by other tools. For now, it just exports the files read.
// Later, this will be expanded to include the Glob and Shell commands, but
// those require a more complicated output format.

pub fn stamp_dump_main() -> Result<()> {
    let mut dump_files = false;
    let mut dump_env = false;
    let mut dump_globs = false;
    let mut dump_cmds = false;
    let mut dump_finds = false;

    let args = std::env::args().skip(2).collect::<Vec<String>>();
    if args.is_empty() {
        anyhow::bail!(
            "Usage: rkati --dump_stamp_tool [--env] [--files] [--globs] [--cmds] [--finds] <stamp>"
        );
    }

    for arg in args[..args.len() - 1].iter() {
        match arg.as_str() {
            "--env" => dump_env = true,
            "--files" => dump_files = true,
            "--globs" => dump_globs = true,
            "--cmds" => dump_cmds = true,
            "--finds" => dump_finds = true,
            _ => {
                anyhow::bail!("Unknown option: {}", arg);
            }
        }
    }

    if !dump_files && !dump_env && !dump_globs && !dump_cmds && !dump_finds {
        dump_files = true;
    }

    let fp = std::fs::File::open(&args[args.len() - 1])?;
    let mut fp = BufReader::new(fp);

    inner(
        &mut fp, dump_files, dump_env, dump_globs, dump_cmds, dump_finds,
    )
    .context("Incomplete stamp file")?;

    Ok(())
}

fn inner(
    fp: &mut impl std::io::Read,
    dump_files: bool,
    dump_env: bool,
    dump_globs: bool,
    dump_cmds: bool,
    dump_finds: bool,
) -> Option<()> {
    let _gen_time = load_systemtime(fp)?;

    //
    // See regen.rs check_step1 for how this is read normally
    //

    {
        let files = load_vec_string(fp)?;
        if dump_files {
            for file in files {
                println!("{}", file.display());
            }
        }
    }

    {
        let undefined = load_vec_string(fp)?;
        if dump_env {
            for var in undefined {
                println!("undefined: {}", var.display());
            }
        }
    }

    let num_envs = load_usize(fp)?;
    for _ in 0..num_envs {
        let name = load_string(fp)?;
        let value = load_string(fp)?;
        if dump_env {
            println!("{}: {}", name.display(), value.display());
        }
    }

    let num_globs = load_usize(fp)?;
    for _ in 0..num_globs {
        let pat = load_string(fp)?;

        let files = load_vec_string(fp)?;
        if dump_globs {
            println!("{}", pat.display());

            for s in files {
                println!("  {}", s.display());
            }
        }
    }

    let num_cmds = load_usize(fp)?;
    for _ in 0..num_cmds {
        let op = CommandOp::from_int(load_int(fp)?)?;
        let shell = load_string(fp)?;
        let shellflag = load_string(fp)?;
        let cmd = load_string(fp)?;
        let result = load_string(fp)?;
        let file = load_string(fp)?;
        let line = load_int(fp)?;
        if line < 0 {
            return None;
        }

        if op == CommandOp::Find {
            let missing_dirs = load_vec_string(fp)?;
            let files = load_vec_string(fp)?;
            let read_dirs = load_vec_string(fp)?;

            if dump_finds {
                println!("cmd type: FIND");
                println!("  shell: {}", shell.display());
                println!("  shell flagss: {}", shellflag.display());
                println!("  loc: {}:{line}", file.display());
                println!("  cmd: {}", cmd.display());
                if !result.is_empty() && result.len() < 500 && !result.as_bytes().contains(&b'\n') {
                    println!("  output: {}", result.display());
                } else {
                    println!("  output: <{} bytes>", result.len());
                }
                println!("  missing dirs:");
                for d in missing_dirs {
                    println!("    {}", d.display());
                }
                println!("  files:");
                for f in files {
                    println!("    {}", f.display());
                }
                println!("  read dirs:");
                for d in read_dirs {
                    println!("    {}", d.display());
                }
                println!();
            }
        } else if dump_cmds {
            match op {
                CommandOp::Shell => {
                    println!("cmd type: SHELL");
                    println!("  shell: {}", shell.display());
                    println!("  shell flagss: {}", shellflag.display());
                }
                CommandOp::Read => println!("cmd type: READ"),
                CommandOp::ReadMissing => println!("cmd type: READ_MISSING"),
                CommandOp::Write => println!("cmd type: WRITE"),
                CommandOp::Append => println!("cmd type: APPEND"),
                CommandOp::Find => unreachable!(),
            }
            println!("  loc: {}:{line}", file.display());
            println!("  cmd: {}", cmd.display());
            if !result.is_empty() && result.len() < 500 && !result.as_bytes().contains(&b'\n') {
                println!("  output: {}", result.display());
            } else {
                println!("  output: <{} bytes>", result.len());
            }
            println!();
        }
    }

    Some(())
}
