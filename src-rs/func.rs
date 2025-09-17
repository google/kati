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
    collections::HashMap,
    ffi::{OsStr, OsString},
    fmt::Debug,
    fs::File,
    io::Write,
    os::unix::ffi::{OsStrExt, OsStringExt},
    sync::{Arc, LazyLock},
};

use anyhow::Result;
use bytes::{Buf, BufMut, Bytes, BytesMut};
use parking_lot::Mutex;

use crate::{
    collect_stats, collect_stats_with_slow_report, error_loc,
    eval::{Evaluator, ExportAllowed, FrameType},
    expr::{Evaluable, Value},
    file_cache::add_extra_file_dep,
    fileutil::{RedirectStderr, run_command},
    find::FindCommand,
    flags::FLAGS,
    kati_warn_loc,
    loc::Loc,
    log,
    parser::parse_buf,
    strutil::{
        Pattern, WordWriter, echo_escape, format_for_command_substitution, has_path_prefix,
        normalize_path, trim_left_space, trim_space, word_scanner,
    },
    symtab::{ScopedGlobalVar, intern},
    var::{VarOrigin, Variable, set_shell_status_var},
    warn_loc,
};

type MakeFuncImpl = fn(&[Arc<Value>], &mut Evaluator, &mut dyn BufMut) -> Result<()>;

pub struct FuncInfo {
    pub name: &'static [u8],
    pub func: MakeFuncImpl,
    pub arity: i16,
    pub min_arity: i16,
    // For all parameters.
    pub trim_space: bool,
    // Only for the first parameter.
    pub trim_right_space_1st: bool,
}

// Function pointers are not comparable, so just compare by name
impl PartialEq for FuncInfo {
    fn eq(&self, other: &Self) -> bool {
        self.name == other.name
    }
}

impl Debug for FuncInfo {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "Func({})", String::from_utf8_lossy(self.name))
    }
}

// TODO: This code is very similar to
// NinjaGenerator::TranslateCommand. Factor them out.
fn strip_shell_comment(cmd: Bytes) -> Bytes {
    if !cmd.contains(&b'#') {
        return cmd;
    }

    let mut res = BytesMut::new();
    let mut prev_backslash = false;
    // Set space as an initial value so the leading comment will be
    // stripped out.
    let mut prev_char = b' ';
    let mut quote = None;
    let mut inp = cmd;
    while !inp.is_empty() {
        let c = inp[0];
        match c {
            b'#' => {
                if quote.is_none() && prev_char.is_ascii_whitespace() {
                    while inp.len() > 1 && !inp.starts_with(b"\n") {
                        inp.advance(1);
                    }
                } else {
                    if let Some(q) = quote {
                        if q == c {
                            quote = None;
                        }
                    } else if !prev_backslash {
                        quote = Some(c);
                    }
                    res.put_u8(c);
                }
            }
            b'\'' | b'"' | b'`' => {
                if let Some(q) = quote {
                    if q == c {
                        quote = None;
                    }
                } else if !prev_backslash {
                    quote = Some(c);
                }
                res.put_u8(c);
            }
            _ => res.put_u8(c),
        }

        if inp.starts_with(b"\\") {
            prev_backslash = !prev_backslash;
        } else {
            prev_backslash = false;
        }

        prev_char = c;
        inp.advance(1);
    }
    res.into()
}

fn patsubst_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let pat_str = args[0].eval_to_buf(ev)?;
    let repl = args[1].eval_to_buf(ev)?;
    let s = args[2].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    let pat = Pattern::new(pat_str);
    for tok in word_scanner(&s) {
        let tok = s.slice_ref(tok);
        ww.write(&pat.append_subst(&tok, &repl));
    }
    Ok(())
}

fn strip_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let s = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&s) {
        ww.write(tok);
    }
    Ok(())
}

fn subst_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let pat = args[0].eval_to_buf(ev)?;
    let repl = args[1].eval_to_buf(ev)?;
    let s = args[2].eval_to_buf(ev)?;
    if pat.is_empty() {
        out.put_slice(&s);
        out.put_slice(&repl);
        return Ok(());
    }
    let f = memchr::memmem::Finder::new(&pat);
    let mut remainder = s.as_ref();
    while !remainder.is_empty() {
        let Some(found) = f.find(remainder) else {
            out.put_slice(remainder);
            break;
        };
        out.put_slice(&remainder[..found]);
        out.put_slice(&repl);
        remainder = &remainder[found + pat.len()..];
    }
    Ok(())
}

fn findstring_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let find = args[0].eval_to_buf(ev)?;
    let f = memchr::memmem::Finder::new(&find);
    let haystack = args[1].eval_to_buf(ev)?;
    if f.find(&haystack).is_some() {
        out.put_slice(&find);
    }
    Ok(())
}

fn filter_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let pat_buf = args[0].eval_to_buf(ev)?;
    let text = args[1].eval_to_buf(ev)?;
    let pats: Vec<Pattern> = word_scanner(&pat_buf)
        .map(|p| Pattern::new(pat_buf.slice_ref(p)))
        .collect();
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        for pat in &pats {
            if pat.matches(tok) {
                ww.write(tok);
                break;
            }
        }
    }
    Ok(())
}

fn filter_out_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let pat_buf = args[0].eval_to_buf(ev)?;
    let text = args[1].eval_to_buf(ev)?;
    let pats: Vec<Pattern> = word_scanner(&pat_buf)
        .map(|p| Pattern::new(pat_buf.slice_ref(p)))
        .collect();
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        let mut matched = false;
        for pat in &pats {
            if pat.matches(tok) {
                matched = true;
                break;
            }
        }
        if !matched {
            ww.write(tok);
        }
    }
    Ok(())
}

fn sort_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let list = args[0].eval_to_buf(ev)?;
    collect_stats!("func sort time");
    let mut toks: Vec<&[u8]> = word_scanner(&list).collect();
    toks.sort();
    let mut ww = WordWriter::new(out);
    let mut prev = [].as_slice();
    for tok in toks {
        if tok != prev {
            ww.write(tok);
            prev = tok;
        }
    }
    Ok(())
}

fn get_numeric_value_for_func(buf: &[u8]) -> Result<usize> {
    let s = std::str::from_utf8(trim_left_space(buf))?;
    Ok(s.parse::<usize>()?)
}

fn word_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let n_str = args[0].eval_to_buf(ev)?;
    let Ok(mut n) = get_numeric_value_for_func(&n_str) else {
        error_loc!(
            ev.loc.as_ref(),
            "*** non-numeric first argument to `word' function: '{}'.",
            String::from_utf8_lossy(&n_str)
        );
    };
    if n == 0 {
        error_loc!(
            ev.loc.as_ref(),
            "*** first argument to `word' function must be greater than 0."
        );
    }

    let text = args[1].eval_to_buf(ev)?;
    for tok in word_scanner(&text) {
        n -= 1;
        if n == 0 {
            out.put_slice(tok);
            break;
        }
    }
    Ok(())
}

fn wordlist_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let s_str = args[0].eval_to_buf(ev)?;
    let Ok(si) = get_numeric_value_for_func(&s_str) else {
        error_loc!(
            ev.loc.as_ref(),
            "*** non-numeric first argument to `wordlist' function: '{}'.",
            String::from_utf8_lossy(&s_str)
        );
    };
    if si == 0 {
        error_loc!(
            ev.loc.as_ref(),
            "*** invalid first argument to `wordlist' function: {}`",
            String::from_utf8_lossy(&s_str)
        );
    }

    let e_str = args[1].eval_to_buf(ev)?;
    let Ok(ei) = get_numeric_value_for_func(&e_str) else {
        error_loc!(
            ev.loc.as_ref(),
            "*** non-numeric second argument to `wordlist' function: '{}'.",
            String::from_utf8_lossy(&e_str)
        );
    };

    let text = args[2].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    let mut i = 0;
    for tok in word_scanner(&text) {
        i += 1;
        if si <= i {
            if i <= ei {
                ww.write(tok);
            } else {
                break;
            }
        }
    }
    Ok(())
}

fn words_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    let n = word_scanner(&text).count();
    out.put_slice(format!("{n}").as_bytes());
    Ok(())
}

fn firstword_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    if let Some(tok) = word_scanner(&text).next() {
        out.put_slice(tok);
    }
    Ok(())
}

fn lastword_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    if let Some(tok) = word_scanner(&text).last() {
        out.put_slice(tok);
    }
    Ok(())
}

fn join_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let list1 = args[0].eval_to_buf(ev)?;
    let list2 = args[1].eval_to_buf(ev)?;
    let mut ws1 = word_scanner(&list1);
    let mut ws2 = word_scanner(&list2);
    let mut ww = WordWriter::new(out);
    loop {
        match (ws1.next(), ws2.next()) {
            (Some(tok1), Some(tok2)) => {
                ww.write(tok1);
                ww.out.put_slice(tok2);
            }
            (Some(tok), None) => ww.write(tok),
            (None, Some(tok)) => ww.write(tok),
            (None, None) => break,
        }
    }
    Ok(())
}

fn wildcard_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let pat = args[0].eval_to_buf(ev)?;
    collect_stats!("func wildcard time");
    // Note GNU make does not delay the execution of $(wildcard) so we
    // do not need to check avoid_io here.
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&pat) {
        let tok = pat.slice_ref(tok);
        let files = crate::fileutil::glob(tok);
        if let Ok(files) = files.as_ref() {
            for f in files {
                ww.write(f);
            }
        }
    }
    Ok(())
}

fn dir_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        let tok = text.slice_ref(tok);
        ww.write(&crate::strutil::dirname(&tok));
        ww.out.put_u8(b'/');
    }
    Ok(())
}

fn notdir_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        if tok == b"/" {
            ww.write(b"");
        } else {
            ww.write(crate::strutil::basename(tok));
        }
    }
    Ok(())
}

fn suffix_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        if let Some(suf) = crate::strutil::get_ext(tok) {
            ww.write(suf);
        }
    }
    Ok(())
}

fn basename_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        ww.write(crate::strutil::strip_ext(tok));
    }
    Ok(())
}

fn addsuffix_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let suf = args[0].eval_to_buf(ev)?;
    let text = args[1].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        ww.write(tok);
        ww.out.put_slice(&suf);
    }
    Ok(())
}

fn addprefix_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let pre = args[0].eval_to_buf(ev)?;
    let text = args[1].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        ww.write(&pre);
        ww.out.put_slice(tok);
    }
    Ok(())
}

fn realpath_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    if ev.avoid_io {
        out.put_slice(b"$(");
        out.put_slice(std::env::current_exe()?.as_os_str().as_bytes());
        out.put_slice(b" --realpath ");
        out.put_slice(&text);
        out.put_slice(b" 2> /dev/null)");
        return Ok(());
    }

    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        let tok = <OsStr as OsStrExt>::from_bytes(tok);
        if let Ok(path) = std::fs::canonicalize(tok) {
            ww.write(path.as_os_str().as_bytes());
        }
    }
    Ok(())
}

fn abspath_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&text) {
        ww.write(&crate::strutil::abs_path(tok)?);
    }
    Ok(())
}

fn if_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let cond = args[0].eval_to_buf(ev)?;
    if cond.is_empty() {
        if args.len() > 2 {
            args[2].eval(ev, out)?;
        }
    } else {
        args[1].eval(ev, out)?;
    }
    Ok(())
}

fn and_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let mut cond = Bytes::new();
    for a in args {
        cond = a.eval_to_buf(ev)?;
        if cond.is_empty() {
            return Ok(());
        }
    }
    if !cond.is_empty() {
        out.put_slice(&cond);
    }
    Ok(())
}

fn or_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    for a in args {
        let cond = a.eval_to_buf(ev)?;
        if !cond.is_empty() {
            out.put_slice(&cond);
            break;
        }
    }
    Ok(())
}

fn value_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let var_name = args[0].eval_to_buf(ev)?;
    let Some(var) = ev.lookup_var(intern(var_name))? else {
        return Ok(());
    };
    out.put_slice(&var.read().string()?);
    Ok(())
}

fn eval_func(args: &[Arc<Value>], ev: &mut Evaluator, _out: &mut dyn BufMut) -> Result<()> {
    let text = args[0].eval_to_buf(ev)?;
    if ev.avoid_io {
        kati_warn_loc!(
            ev.loc.as_ref(),
            "*warning*: $(eval) in a recipe is not recommended: {}",
            String::from_utf8_lossy(&text)
        );
    }
    let stmts = parse_buf(&text, ev.loc.clone().unwrap_or_default())?;
    let stmts = stmts.lock();
    for stmt in stmts.iter() {
        log!("{:?}", stmt);
        stmt.eval(ev)?;
    }
    Ok(())
}

// A hack for Android build. We need to evaluate things like $((3+4))
// when we emit ninja file, because the result of such expressions
// will be passed to other make functions.
// TODO: Maybe we should introduce a helper binary which evaluate
// make expressions at ninja-time.
fn has_no_io_in_shell_script(cmd: &[u8]) -> bool {
    if cmd.is_empty() {
        return true;
    }
    if cmd.starts_with(b"echo $((") && cmd.ends_with(b")") {
        return true;
    }
    false
}

fn shell_func_impl(
    shell: &[u8],
    shellflag: &[u8],
    cmd: &Bytes,
    loc: &Loc,
) -> Result<(i32, Bytes, Option<FindCommand>)> {
    log!("ShellFunc: {:?}", cmd);

    if FLAGS.use_find_emulator
        && let Some(fc) = crate::find::parse(cmd)?
        && let Some(out) = crate::find::find(cmd, &fc, loc)?
    {
        return Ok((0, out, Some(fc)));
    }

    collect_stats_with_slow_report!("func shell time", OsStr::from_bytes(cmd));
    let (status, output) = run_command(shell, shellflag, cmd, RedirectStderr::None)?;
    let output = Bytes::from(format_for_command_substitution(output));

    if let Some(exit_code) = status.code() {
        return Ok((exit_code, output, None));
    }
    let exit_code = if status.success() { 0 } else { 1 };
    Ok((exit_code, output, None))
}

fn should_store_command_result(cmd: &[u8]) -> bool {
    // We really just want to ignore this one, or remove BUILD_DATETIME from
    // Android completely
    if cmd == b"date +%s" {
        return false;
    }

    if let Some(pat) = &FLAGS.ignore_dirty_pattern {
        let nopat = &FLAGS.no_ignore_dirty_pattern;
        for tok in word_scanner(cmd) {
            if pat.matches(tok) && !nopat.as_ref().map(|p| p.matches(tok)).unwrap_or(false) {
                return false;
            }
        }
    }

    true
}

pub static COMMAND_RESULTS: LazyLock<Mutex<Vec<CommandResult>>> =
    LazyLock::new(|| Mutex::new(Vec::new()));

#[derive(PartialEq, Eq, Clone, Copy)]
pub enum CommandOp {
    Shell,
    Find,
    Read,
    ReadMissing,
    Write,
    Append,
}

impl CommandOp {
    pub fn as_int(&self) -> i32 {
        match self {
            CommandOp::Shell => 0,
            CommandOp::Find => 1,
            CommandOp::Read => 2,
            CommandOp::ReadMissing => 3,
            CommandOp::Write => 4,
            CommandOp::Append => 5,
        }
    }

    pub fn from_int(i: i32) -> Option<CommandOp> {
        match i {
            0 => Some(CommandOp::Shell),
            1 => Some(CommandOp::Find),
            2 => Some(CommandOp::Read),
            3 => Some(CommandOp::ReadMissing),
            4 => Some(CommandOp::Write),
            5 => Some(CommandOp::Append),
            _ => None,
        }
    }
}

pub struct CommandResult {
    pub op: CommandOp,
    pub shell: Bytes,
    pub shellflag: Bytes,
    pub cmd: Bytes,
    pub find: Option<FindCommand>,
    pub result: Bytes,
    pub loc: Loc,
}

fn shell_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let cmd = args[0].eval_to_buf(ev)?;
    if ev.avoid_io && !has_no_io_in_shell_script(&cmd) {
        if ev.eval_depth > 1 {
            error_loc!(
                ev.loc.as_ref(),
                "kati doesn't support passing results of $(shell) to other make constructs: {}",
                String::from_utf8_lossy(&cmd)
            );
        }
        let cmd = strip_shell_comment(cmd);
        out.put_slice(b"$(");
        out.put_slice(&cmd);
        out.put_u8(b')');
        return Ok(());
    }

    let loc = ev.loc.clone().unwrap_or_default();
    let shell = ev.get_shell()?;
    let shellflag = ev.get_shell_flag();

    let (exit_code, output, fc) = shell_func_impl(&shell, shellflag, &cmd, &loc)?;
    out.put_slice(&output);
    if should_store_command_result(&cmd) {
        COMMAND_RESULTS.lock().push(CommandResult {
            op: if fc.is_some() {
                CommandOp::Find
            } else {
                CommandOp::Shell
            },
            shell,
            shellflag: Bytes::from_static(shellflag),
            cmd,
            find: fc,
            result: output,
            loc,
        })
    }
    set_shell_status_var(exit_code);
    Ok(())
}

fn shell_no_rerun_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    out: &mut dyn BufMut,
) -> Result<()> {
    let cmd = args[0].eval_to_buf(ev)?;
    if ev.avoid_io && !has_no_io_in_shell_script(&cmd) {
        // In the regular ShellFunc, if it sees a $(shell) inside of a rule when in
        // ninja mode, the shell command will just be written to the ninja file
        // instead of run directly by kati. So it already has the benefits of not
        // rerunning every time kati is invoked.
        error_loc!(
            ev.loc.as_ref(),
            "KATI_shell_no_rerun provides no benefit over regular $(shell) inside of a rule."
        );
    }

    let loc = ev.loc.clone().unwrap_or_default();
    let shell = ev.get_shell()?;
    let shellflag = ev.get_shell_flag();

    let (exit_code, output, _) = shell_func_impl(&shell, shellflag, &cmd, &loc)?;
    out.put_slice(&output);
    set_shell_status_var(exit_code);
    Ok(())
}

fn call_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let func_name_buf = args[0].eval_to_buf(ev)?;
    let func_name_buf = func_name_buf.slice_ref(trim_space(&func_name_buf));
    let func_sym = intern(func_name_buf.clone());
    let func = ev.lookup_var(func_sym)?;
    if let Some(func) = &func {
        let func = func.read();
        func.used(ev, &func_sym)?;
    } else if FLAGS.enable_kati_warnings {
        kati_warn_loc!(
            ev.loc.as_ref(),
            "*warning*: undefined user function: {func_sym}"
        );
    }
    let mut av = Vec::with_capacity(args.len() - 1);
    for arg in &args[1..] {
        av.push(Variable::with_simple_string(
            arg.eval_to_buf(ev)?,
            VarOrigin::Automatic,
            None,
            None,
        ));
    }
    let mut sv = Vec::new();
    let mut i = 1;
    loop {
        let tmpvar_name_sym = intern(format!("{i}"));
        if let Some(a) = av.get(i - 1) {
            sv.push(ScopedGlobalVar::new(tmpvar_name_sym, a.clone())?);
        } else {
            // We need to blank further automatic vars
            let Some(v) = ev.lookup_var(tmpvar_name_sym)? else {
                break;
            };
            if v.read().origin() != VarOrigin::Automatic {
                break;
            }

            let v = Variable::new_simple(VarOrigin::Automatic, None, None);
            sv.push(ScopedGlobalVar::new(tmpvar_name_sym, v)?);
        }
        i += 1;
    }

    ev.eval_depth -= 1;

    {
        let _frame = ev.enter(
            FrameType::Call,
            func_name_buf,
            ev.loc.clone().unwrap_or_default(),
        );
        if let Some(func) = func {
            func.read().eval(ev, out)?;
        }
    }

    ev.eval_depth += 1;

    Ok(())
}

fn foreach_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let varname = intern(args[0].eval_to_buf(ev)?);
    let list = args[1].eval_to_buf(ev)?;
    ev.eval_depth -= 1;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&list) {
        let tok = list.slice_ref(tok);
        let v = Variable::with_simple_string(tok, VarOrigin::Automatic, None, None);
        let _sv = ScopedGlobalVar::new(varname, v)?;
        ww.maybe_add_space();
        args[2].eval(ev, ww.out)?;
    }
    ev.eval_depth += 1;
    Ok(())
}

fn origin_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let var_name = args[0].eval_to_buf(ev)?;
    if let Some(var) = ev.lookup_var(intern(var_name))? {
        let orig = var.read().origin();
        out.put_slice(crate::var::get_origin_str(orig).as_bytes());
    } else {
        out.put_slice(b"undefined");
    }
    Ok(())
}

fn flavor_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let var_name = args[0].eval_to_buf(ev)?;
    if let Some(var) = ev.lookup_var(intern(var_name))? {
        out.put_slice(var.read().flavor().as_bytes());
    } else {
        out.put_slice(b"undefined");
    }
    Ok(())
}

fn info_func(args: &[Arc<Value>], ev: &mut Evaluator, _out: &mut dyn BufMut) -> Result<()> {
    let a = args[0].eval_to_buf(ev)?;
    if ev.avoid_io {
        let mut s = BytesMut::new();
        s.put_slice(b"echo -e \"");
        s.put_slice(&echo_escape(&a));
        s.put_u8(b'"');
        ev.delayed_output_commands.push(s.freeze());
    } else {
        println!("{}", String::from_utf8_lossy(&a));
    }
    Ok(())
}

fn warning_func(args: &[Arc<Value>], ev: &mut Evaluator, _out: &mut dyn BufMut) -> Result<()> {
    let a = args[0].eval_to_buf(ev)?;
    if ev.avoid_io {
        let mut s = BytesMut::new();
        s.put_slice(b"echo -e \"");
        s.put_slice(ev.loc.clone().unwrap_or_default().to_string().as_bytes());
        s.put_slice(b": ");
        s.put_slice(&echo_escape(&a));
        s.put_slice(b"\" 2>&1");
        ev.delayed_output_commands.push(s.freeze());
        return Ok(());
    }
    warn_loc!(ev.loc.as_ref(), "{}", String::from_utf8_lossy(&a));
    Ok(())
}

fn error_func(args: &[Arc<Value>], ev: &mut Evaluator, _out: &mut dyn BufMut) -> Result<()> {
    let a = args[0].eval_to_buf(ev)?;
    if ev.avoid_io {
        let mut s = BytesMut::new();
        s.put_slice(b"echo -e \"");
        s.put_slice(ev.loc.clone().unwrap_or_default().to_string().as_bytes());
        s.put_slice(b": *** ");
        s.put_slice(&echo_escape(&a));
        s.put_slice(b".\" 2>&1 && false");
        ev.delayed_output_commands.push(s.freeze());
        return Ok(());
    }
    error_loc!(ev.loc.as_ref(), "*** {}.", String::from_utf8_lossy(&a));
}

fn file_read_func(
    ev: &mut Evaluator,
    filename: &OsStr,
    out: &mut dyn BufMut,
    rerun: bool,
) -> Result<()> {
    if !std::fs::exists(filename)? {
        if should_store_command_result(filename.as_bytes()) {
            COMMAND_RESULTS.lock().push(CommandResult {
                op: CommandOp::ReadMissing,
                shell: Bytes::new(),
                shellflag: Bytes::new(),
                cmd: Bytes::from(filename.as_bytes().to_vec()),
                find: None,
                result: Bytes::new(),
                loc: ev.loc.clone().unwrap_or_default(),
            })
        }
        return Ok(());
    }

    let mut buf = std::fs::read(filename)?;
    if buf.ends_with(b"\n") {
        buf.pop();
    }
    let buf = Bytes::from(buf);

    if rerun && should_store_command_result(filename.as_bytes()) {
        COMMAND_RESULTS.lock().push(CommandResult {
            op: CommandOp::Read,
            shell: Bytes::new(),
            shellflag: Bytes::new(),
            cmd: Bytes::from(filename.as_bytes().to_vec()),
            find: None,
            result: buf.clone(),
            loc: ev.loc.clone().unwrap_or_default(),
        })
    }
    out.put_slice(&buf);
    Ok(())
}

fn file_write_func(
    ev: &mut Evaluator,
    filename: &OsStr,
    append: bool,
    text: Bytes,
    rerun: bool,
) -> Result<()> {
    {
        let mut f = File::options()
            .write(true)
            .append(append)
            .truncate(!append)
            .create(true)
            .open(filename)?;
        f.write_all(&text)?;
    }

    if rerun && should_store_command_result(filename.as_bytes()) {
        COMMAND_RESULTS.lock().push(CommandResult {
            op: CommandOp::Write,
            shell: Bytes::new(),
            shellflag: Bytes::new(),
            cmd: Bytes::from(filename.as_bytes().to_vec()),
            find: None,
            result: text,
            loc: ev.loc.clone().unwrap_or_default(),
        })
    }

    Ok(())
}

fn file_func_impl(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    out: &mut dyn BufMut,
    rerun: bool,
) -> Result<()> {
    if ev.avoid_io {
        error_loc!(
            ev.loc.as_ref(),
            "*** $(file ...) is not supported in rules."
        );
    }

    let arg = args[0].eval_to_buf(ev)?;
    let filename = trim_space(&arg);

    if filename.is_empty() {
        error_loc!(ev.loc.as_ref(), "*** Missing filename");
    }

    if filename[0] == b'<' {
        let filename = trim_left_space(&filename[1..]);
        if filename.is_empty() {
            error_loc!(ev.loc.as_ref(), "*** Missing filename");
        }
        if args.len() > 1 {
            error_loc!(ev.loc.as_ref(), "*** invalid argument");
        }

        let filename = <OsStr as OsStrExt>::from_bytes(filename);
        file_read_func(ev, filename, out, rerun)?;
    } else if filename[0] == b'>' {
        let append = filename.starts_with(b">>");
        let filename = trim_left_space(&filename[if append { 2 } else { 1 }..]);
        if filename.is_empty() {
            error_loc!(ev.loc.as_ref(), "*** Missing filename");
        }

        let mut text = BytesMut::new();
        if let Some(contents) = args.get(1) {
            contents.eval(ev, &mut text)?;
            if text.is_empty() || !text.ends_with(b"\n") {
                text.put_u8(b'\n');
            }
        }

        let filename = <OsStr as OsStrExt>::from_bytes(filename);
        file_write_func(ev, filename, append, text.freeze(), rerun)?;
    } else {
        error_loc!(
            ev.loc.as_ref(),
            "*** Invalid file operation: {}.  Stop.",
            String::from_utf8_lossy(filename)
        );
    }
    Ok(())
}

fn file_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    file_func_impl(args, ev, out, true)
}

fn file_no_rerun_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    file_func_impl(args, ev, out, false)
}

fn deprecated_var_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    _out: &mut dyn BufMut,
) -> Result<()> {
    let vars_str = args[0].eval_to_buf(ev)?;
    let msg = Arc::new(if let Some(v) = args.get(1) {
        format!(". {}", String::from_utf8_lossy(&v.eval_to_buf(ev)?))
    } else {
        String::new()
    });

    if ev.avoid_io {
        error_loc!(
            ev.loc.as_ref(),
            "*** $(KATI_deprecated_var ...) is not supported in rules."
        );
    }

    for var in word_scanner(&vars_str) {
        let var = vars_str.slice_ref(var);
        let sym = intern(var);
        let v = match ev.peek_var(sym) {
            Some(v) => v,
            None => {
                let v =
                    Variable::new_simple(VarOrigin::File, Some(ev.current_frame()), ev.loc.clone());
                sym.set_global_var(v.clone(), false, None)?;
                v
            }
        };

        let mut v = v.write();
        if v.deprecated.is_some() {
            error_loc!(
                ev.loc.as_ref(),
                "*** Cannot call KATI_deprecated_var on already deprecated variable: {sym}."
            );
        } else if v.obsolete() {
            error_loc!(
                ev.loc.as_ref(),
                "*** Cannot call KATI_deprecated_var on already obsolete variable: {sym}."
            );
        }

        v.deprecated = Some(msg.clone());
    }
    Ok(())
}

fn obsolete_var_func(args: &[Arc<Value>], ev: &mut Evaluator, _out: &mut dyn BufMut) -> Result<()> {
    let vars_str = args[0].eval_to_buf(ev)?;
    let msg = Arc::new(if let Some(v) = args.get(1) {
        format!(". {}", String::from_utf8_lossy(&v.eval_to_buf(ev)?))
    } else {
        String::new()
    });

    if ev.avoid_io {
        error_loc!(
            ev.loc.as_ref(),
            "*** $(KATI_obsolete_var ...) is not supported in rules."
        );
    }

    for var in word_scanner(&vars_str) {
        let var = vars_str.slice_ref(var);
        let sym = intern(var);
        let v = match ev.peek_var(sym) {
            Some(v) => v,
            None => {
                let v =
                    Variable::new_simple(VarOrigin::File, Some(ev.current_frame()), ev.loc.clone());
                sym.set_global_var(v.clone(), false, None)?;
                v
            }
        };

        let mut v = v.write();
        if v.deprecated.is_some() {
            error_loc!(
                ev.loc.as_ref(),
                "*** Cannot call KATI_obsolete_var on already deprecated variable: {sym}."
            );
        } else if v.obsolete() {
            error_loc!(
                ev.loc.as_ref(),
                "*** Cannot call KATI_obsolete_var on already obsolete variable: {sym}."
            );
        }

        v.set_obsolete(msg.clone());
    }
    Ok(())
}

fn deprecate_export_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    _out: &mut dyn BufMut,
) -> Result<()> {
    let msg = format!(". {}", String::from_utf8_lossy(&args[0].eval_to_buf(ev)?));

    if ev.avoid_io {
        error_loc!(
            ev.loc.as_ref(),
            "*** $(KATI_deprecate_export) is not supported in rules."
        );
    }

    match &ev.export_allowed {
        ExportAllowed::Warning(_) => {
            error_loc!(ev.loc.as_ref(), "*** Export is already deprecated.")
        }
        ExportAllowed::Error(_) => error_loc!(ev.loc.as_ref(), "*** Export is already obsolete."),
        ExportAllowed::Allowed => {}
    }

    ev.export_allowed = ExportAllowed::Warning(msg);
    Ok(())
}

fn obsolete_export_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    _out: &mut dyn BufMut,
) -> Result<()> {
    let msg = format!(". {}", String::from_utf8_lossy(&args[0].eval_to_buf(ev)?));

    if ev.avoid_io {
        error_loc!(
            ev.loc.as_ref(),
            "*** $(KATI_obsolete_export) is not supported in rules."
        );
    }

    if matches!(ev.export_allowed, ExportAllowed::Error(_)) {
        error_loc!(ev.loc.as_ref(), "*** Export is already obsolete.");
    }

    ev.export_allowed = ExportAllowed::Error(msg);
    Ok(())
}

fn profile_makefile_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    _out: &mut dyn BufMut,
) -> Result<()> {
    for arg in args {
        let files = arg.eval_to_buf(ev)?;
        for file in word_scanner(&files) {
            ev.profiled_files.push(OsString::from_vec(file.to_vec()));
        }
    }
    Ok(())
}

fn variable_location_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    out: &mut dyn BufMut,
) -> Result<()> {
    let arg = args[0].eval_to_buf(ev)?;
    let mut ww = WordWriter::new(out);
    for var in word_scanner(&arg) {
        let var = arg.slice_ref(var);
        let sym = intern(var);
        let l = ev
            .peek_var(sym)
            .and_then(|v| v.read().loc().clone())
            .unwrap_or_default();
        ww.write(l.to_string().as_bytes());
    }
    Ok(())
}

fn extra_file_deps_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    _out: &mut dyn BufMut,
) -> Result<()> {
    for arg in args {
        let files = arg.eval_to_buf(ev)?;
        for file in word_scanner(&files) {
            let fname = <OsStr as OsStrExt>::from_bytes(file);
            if !std::fs::exists(fname)? {
                error_loc!(
                    ev.loc.as_ref(),
                    "*** file does not exist: {}",
                    fname.to_string_lossy()
                );
            }
            add_extra_file_dep(fname.to_os_string());
        }
    }
    Ok(())
}

fn foreach_sep_func(args: &[Arc<Value>], ev: &mut Evaluator, out: &mut dyn BufMut) -> Result<()> {
    let varname = intern(args[0].eval_to_buf(ev)?);
    let separator = args[1].eval_to_buf(ev)?;
    let list = args[2].eval_to_buf(ev)?;
    ev.eval_depth -= 1;
    let mut ww = WordWriter::new(out);
    for tok in word_scanner(&list) {
        let tok = list.slice_ref(tok);
        let v = Variable::with_simple_string(tok, VarOrigin::Automatic, None, None);
        let _sv = ScopedGlobalVar::new(varname, v)?;
        ww.maybe_add_separator(&separator);
        args[3].eval(ev, ww.out)?;
    }
    ev.eval_depth += 1;
    Ok(())
}

fn visibility_prefix_func(
    args: &[Arc<Value>],
    ev: &mut Evaluator,
    _out: &mut dyn BufMut,
) -> Result<()> {
    let arg = args[0].eval_to_buf(ev)?;
    let mut prefixes: Vec<OsString> = Vec::new();

    for prefix in word_scanner(&args[1].eval_to_buf(ev)?) {
        if prefix.starts_with(b"/") {
            error_loc!(ev.loc.as_ref(), "Visibility prefix should not start with /");
        }
        if prefix.starts_with(b"../") {
            error_loc!(
                ev.loc.as_ref(),
                "Visibility prefix should not start with ../"
            );
        }

        let normalized_prefix = normalize_path(prefix);
        if prefix != normalized_prefix {
            error_loc!(
                ev.loc.as_ref(),
                "Visibility prefix {} is not normalized. Normalized prefix: {}",
                String::from_utf8_lossy(prefix),
                String::from_utf8_lossy(&normalized_prefix)
            );
        }

        // one visibility prefix cannot be the prefix of another visibility prefix
        for p in &prefixes {
            if has_path_prefix(p.as_bytes(), prefix) {
                error_loc!(
                    ev.loc.as_ref(),
                    "Visibility prefix {} is the prefix of another visibility prefix {}",
                    String::from_utf8_lossy(prefix),
                    p.to_string_lossy(),
                );
            } else if has_path_prefix(prefix, p.as_bytes()) {
                error_loc!(
                    ev.loc.as_ref(),
                    "Visibility prefix {} is the prefix of another visibility prefix {}",
                    p.to_string_lossy(),
                    String::from_utf8_lossy(prefix),
                );
            }
        }

        prefixes.push(OsStringExt::from_vec(normalized_prefix.to_vec()));
    }

    let sym = intern(arg);
    let v = if let Some(v) = ev.peek_var(sym) {
        v
    } else {
        // If variable is not defined, create an empty variable.
        let v = Variable::new_simple(VarOrigin::File, Some(ev.current_frame()), ev.loc.clone());
        sym.set_global_var(v.clone(), false, None)?;
        v
    };
    if !prefixes.is_empty() {
        v.write().set_visibility_prefix(prefixes, &sym)?;
    }

    Ok(())
}

fn debug_func(args: &[Arc<Value>], ev: &mut Evaluator, _out: &mut dyn BufMut) -> Result<()> {
    let a = args[0].eval_to_buf(ev)?;
    let loc = ev.loc.clone().unwrap_or_default();
    for tok in word_scanner(&a) {
        let tok = a.slice_ref(tok);
        let tok = intern(tok);
        let Some(v) = ev.lookup_var(tok)? else {
            println!("{loc}: Variable {tok:?} is undefined");
            continue;
        };
        let v = v.read();
        let val = v.eval_to_buf(ev)?;
        println!("{loc}: Variable {tok:?}={val:?} ({v:?})")
    }
    Ok(())
}

const fn func(name: &'static [u8], f: MakeFuncImpl, arity: i16) -> FuncInfo {
    FuncInfo {
        name,
        func: f,
        arity,
        min_arity: arity,
        trim_space: false,
        trim_right_space_1st: false,
    }
}
const FUNC_INFO: &[FuncInfo] = &[
    func(b"patsubst", patsubst_func, 3),
    func(b"strip", strip_func, 1),
    func(b"subst", subst_func, 3),
    func(b"findstring", findstring_func, 2),
    func(b"filter", filter_func, 2),
    func(b"filter-out", filter_out_func, 2),
    func(b"sort", sort_func, 1),
    func(b"word", word_func, 2),
    func(b"wordlist", wordlist_func, 3),
    func(b"words", words_func, 1),
    func(b"firstword", firstword_func, 1),
    func(b"lastword", lastword_func, 1),
    func(b"join", join_func, 2),
    func(b"wildcard", wildcard_func, 1),
    func(b"dir", dir_func, 1),
    func(b"notdir", notdir_func, 1),
    func(b"suffix", suffix_func, 1),
    func(b"basename", basename_func, 1),
    func(b"addsuffix", addsuffix_func, 2),
    func(b"addprefix", addprefix_func, 2),
    func(b"realpath", realpath_func, 1),
    func(b"abspath", abspath_func, 1),
    FuncInfo {
        name: b"if",
        func: if_func,
        arity: 3,
        min_arity: 2,
        trim_space: false,
        trim_right_space_1st: true,
    },
    FuncInfo {
        name: b"and",
        func: and_func,
        arity: 0,
        min_arity: 0,
        trim_space: true,
        trim_right_space_1st: false,
    },
    FuncInfo {
        name: b"or",
        func: or_func,
        arity: 0,
        min_arity: 0,
        trim_space: true,
        trim_right_space_1st: false,
    },
    func(b"value", value_func, 1),
    func(b"eval", eval_func, 1),
    func(b"shell", shell_func, 1),
    func(b"call", call_func, 0),
    func(b"foreach", foreach_func, 3),
    func(b"origin", origin_func, 1),
    func(b"flavor", flavor_func, 1),
    func(b"info", info_func, 1),
    func(b"warning", warning_func, 1),
    func(b"error", error_func, 1),
    FuncInfo {
        name: b"file",
        func: file_func,
        arity: 2,
        min_arity: 1,
        trim_space: false,
        trim_right_space_1st: false,
    },
    /* Kati custom extension functions */
    FuncInfo {
        name: b"KATI_deprecated_var",
        func: deprecated_var_func,
        arity: 2,
        min_arity: 1,
        trim_space: false,
        trim_right_space_1st: false,
    },
    FuncInfo {
        name: b"KATI_obsolete_var",
        func: obsolete_var_func,
        arity: 2,
        min_arity: 1,
        trim_space: false,
        trim_right_space_1st: false,
    },
    func(b"KATI_deprecate_export", deprecate_export_func, 1),
    func(b"KATI_obsolete_export", obsolete_export_func, 1),
    func(b"KATI_profile_makefile", profile_makefile_func, 0),
    func(b"KATI_variable_location", variable_location_func, 1),
    func(b"KATI_extra_file_deps", extra_file_deps_func, 0),
    func(b"KATI_shell_no_rerun", shell_no_rerun_func, 1),
    func(b"KATI_foreach_sep", foreach_sep_func, 4),
    FuncInfo {
        name: b"KATI_file_no_rerun",
        func: file_no_rerun_func,
        arity: 2,
        min_arity: 1,
        trim_space: false,
        trim_right_space_1st: false,
    },
    FuncInfo {
        name: b"KATI_visibility_prefix",
        func: visibility_prefix_func,
        arity: 2,
        min_arity: 1,
        trim_space: false,
        trim_right_space_1st: false,
    },
    func(b"KATI_debug_var", debug_func, 1),
];

static FUNC_INFO_MAP: LazyLock<HashMap<&'static [u8], &'static FuncInfo>> =
    LazyLock::new(|| FUNC_INFO.iter().map(|f| (f.name, f)).collect());

pub fn get_func_info(name: &[u8]) -> Option<&'static FuncInfo> {
    FUNC_INFO_MAP.get(name).map(|v| &**v)
}
