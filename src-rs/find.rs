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
    collections::{HashMap, HashSet},
    ffi::{CString, OsStr, OsString},
    os::unix::{ffi::OsStrExt, fs::FileTypeExt},
    path::PathBuf,
    sync::{Arc, LazyLock, OnceLock, Weak, atomic::AtomicUsize},
};

use anyhow::Result;
use bytes::{Buf, Bytes, BytesMut};
use libc::FNM_PERIOD;
use memchr::{memchr, memchr3};
use parking_lot::Mutex;

use crate::{
    collect_stats, error,
    fileutil::fnmatch,
    flags::FLAGS,
    loc::Loc,
    log,
    strutil::{WordWriter, basename, concat_dir, has_word, normalize_path, trim_left_space},
    warn,
};

static NODE_COUNT: AtomicUsize = AtomicUsize::new(0);

macro_rules! find_warn_loc {
    ($loc:expr, $fmt:expr $(, $($arg:tt)*)?) => {
        if FLAGS.werror_find_emulator {
            crate::error_loc!($loc, $fmt, $($($arg)*)?)
        } else {
            crate::warn_loc!($loc, $fmt, $($($arg)*)?)
        }
    };
}

#[derive(PartialEq, Eq, Debug)]
pub enum FindCommandType {
    Find,
    FindLeaves,
    Ls,
}

#[derive(PartialEq, Eq, Debug, Clone, Copy)]
enum FileType {
    BlockDevice,
    CharDevice,
    Dir,
    Fifo,
    Symlink,
    Regular,
    Socket,
}

impl TryFrom<std::fs::FileType> for FileType {
    type Error = anyhow::Error;

    fn try_from(ft: std::fs::FileType) -> Result<Self, Self::Error> {
        if ft.is_file() {
            Ok(FileType::Regular)
        } else if ft.is_dir() {
            Ok(FileType::Dir)
        } else if ft.is_symlink() {
            Ok(FileType::Symlink)
        } else if ft.is_fifo() {
            Ok(FileType::Fifo)
        } else if ft.is_socket() {
            Ok(FileType::Socket)
        } else if ft.is_char_device() {
            Ok(FileType::CharDevice)
        } else if ft.is_block_device() {
            Ok(FileType::BlockDevice)
        } else {
            anyhow::bail!("Unsupported file type: {ft:?}")
        }
    }
}

#[derive(PartialEq, Eq, Debug)]
enum FindCond {
    Name { pat: CString, has_wildcard: bool },
    Typ(FileType),
    Not(Box<FindCond>),
    And(Box<FindCond>, Box<FindCond>),
    Or(Box<FindCond>, Box<FindCond>),
}

impl FindCond {
    fn new_name(name: &[u8]) -> Result<Self> {
        Ok(FindCond::Name {
            pat: CString::new(name)?,
            has_wildcard: memchr3(b'?', b'*', b'[', name).is_some(),
        })
    }
    fn new_type(t: FileType) -> Self {
        FindCond::Typ(t)
    }
    fn new_not(cond: FindCond) -> Self {
        FindCond::Not(Box::new(cond))
    }
    fn new_and(cond1: FindCond, cond2: FindCond) -> Self {
        FindCond::And(Box::new(cond1), Box::new(cond2))
    }
    fn new_or(cond1: FindCond, cond2: FindCond) -> Self {
        FindCond::Or(Box::new(cond1), Box::new(cond2))
    }

    fn is_true(&self, path: &[u8], typ: FileType) -> bool {
        match self {
            FindCond::Name { pat, .. } => fnmatch(pat, basename(path), 0),
            FindCond::Typ(t) => typ == *t,
            FindCond::Not(cond) => !cond.is_true(path, typ),
            FindCond::And(cond1, cond2) => cond1.is_true(path, typ) && cond2.is_true(path, typ),
            FindCond::Or(cond1, cond2) => cond1.is_true(path, typ) || cond2.is_true(path, typ),
        }
    }
    fn countable(&self) -> bool {
        match self {
            FindCond::Name { has_wildcard, .. } => !has_wildcard,
            FindCond::Or(a, b) => a.countable() && b.countable(),
            _ => false,
        }
    }
    fn count(&self) -> usize {
        match self {
            FindCond::Name { .. } => 1,
            FindCond::Or(a, b) => a.count() + b.count(),
            _ => 0,
        }
    }
}

struct DirentNode {
    base: OsString,
    inner: OnceLock<NodeType>,
    init_data: Mutex<Option<NodeTypeInitData>>,
}
enum NodeType {
    File {
        typ: FileType,
    },
    Dir {
        parent: Option<Weak<DirentNode>>,
        children: Vec<(OsString, Arc<DirentNode>)>,
    },
    Symlink {
        // If there is a loop (a symlink to .., etc), this may leak. But
        // it's not the end of the world if this doesn't get freed.
        to: Arc<DirentNode>,
    },
    SymlinkError {
        err: std::io::Error,
    },
    UnsupportedSymlink {},
    Error {},
}
enum NodeTypeInitData {
    Dir {
        name: PathBuf,
        parent: Option<Weak<DirentNode>>,
    },
    Symlink {
        name: PathBuf,
        parent: Weak<DirentNode>,
    },
}
impl DirentNode {
    fn new() -> Arc<DirentNode> {
        Arc::new(DirentNode {
            base: "".into(),
            inner: OnceLock::new(),
            init_data: Mutex::new(Some(NodeTypeInitData::Dir {
                name: std::env::current_dir().unwrap(),
                parent: None,
            })),
        })
    }
    fn print_if_necessary(
        &self,
        fc: &FindCommand,
        path: &[u8],
        typ: FileType,
        d: i32,
        out: &mut Vec<Vec<u8>>,
    ) {
        if let Some(print_cond) = &fc.print_cond
            && !print_cond.is_true(path, typ)
        {
            return;
        }
        if d < fc.mindepth {
            return;
        }
        out.push(path.to_vec())
    }
    fn is_directory(self: &Arc<Self>) -> bool {
        match self.inner(true) {
            Some(NodeType::Error {}) => false,
            Some(NodeType::File { .. }) => false,
            Some(NodeType::Dir { .. }) => true,
            Some(NodeType::SymlinkError { .. }) => false,
            Some(NodeType::UnsupportedSymlink {}) => false,
            Some(NodeType::Symlink { to, .. }) => to.is_directory(),
            None => false,
        }
    }
    fn find_dir(self: &Arc<Self>, d: &[u8]) -> Option<Arc<DirentNode>> {
        match self.inner(true) {
            Some(NodeType::Dir { parent, children }) => {
                if d.is_empty() || d == b"." {
                    return Some(self.clone());
                }
                if d == b".." {
                    return parent.clone()?.upgrade();
                }

                let idx = memchr(b'/', d);
                let (p, rest) = if let Some(idx) = idx {
                    (&d[..idx], &d[idx + 1..])
                } else {
                    (d, &[][..])
                };
                if p.is_empty() || p == b"." {
                    return self.find_dir(rest);
                }
                if p == b".." {
                    let parent = parent.clone()?.upgrade()?;
                    return parent.find_dir(rest);
                }

                let p = OsStr::from_bytes(p);
                for (base, child) in children {
                    if p == base {
                        if idx.is_none() {
                            return Some(child.clone());
                        }
                        return child.find_dir(rest);
                    }
                }
                None
            }
            Some(NodeType::Symlink { to }) => to.find_dir(d),
            _ => None,
        }
    }
    fn find_nodes(
        self: &Arc<Self>,
        fc: &FindCommand,
        results: &mut Vec<(Vec<u8>, Arc<DirentNode>)>,
        path: &mut Vec<u8>,
        d: &[u8],
    ) -> bool {
        match self.inner(true) {
            Some(NodeType::Dir { parent, children }) => {
                if !path.is_empty() {
                    path.push(b'/');
                }

                let orig_path_size = path.len();

                let idx = memchr(b'/', d);
                let (p, rest) = if let Some(idx) = idx {
                    (&d[..idx], &d[idx + 1..])
                } else {
                    (d, &[][..])
                };

                if p.is_empty() || p == b"." {
                    path.extend_from_slice(p);
                    if idx.is_none() {
                        results.push((path.clone(), self.clone()));
                        return true;
                    }
                    return self.find_nodes(fc, results, path, rest);
                }
                if p == b".." {
                    let Some(parent) = parent.clone().and_then(|p| p.upgrade()) else {
                        log!(
                            "FindEmulator does not support leaving the source directory: {}",
                            String::from_utf8_lossy(path)
                        );
                        return false;
                    };
                    path.extend_from_slice(p);
                    if idx.is_none() {
                        results.push((path.clone(), parent));
                        return true;
                    }
                    return parent.find_nodes(fc, results, path, rest);
                }

                let is_wild = memchr3(b'?', b'*', b'[', p).is_some();
                if is_wild {
                    let mut v = fc.read_dirs.lock();
                    v.insert(Bytes::from(path.clone()));
                }
                let pattern = if is_wild { CString::new(p).ok() } else { None };

                for (name, child) in children {
                    let name = name.as_bytes();
                    let matches = if let Some(pattern) = &pattern {
                        fnmatch(pattern, name, FNM_PERIOD)
                    } else {
                        p == name
                    };
                    if matches {
                        path.extend_from_slice(name);
                        if idx.is_none() {
                            results.push((path.clone(), child.clone()));
                        } else if !child.find_nodes(fc, results, path, rest) {
                            return false;
                        }
                        path.truncate(orig_path_size);
                    }
                }

                true
            }
            Some(NodeType::Symlink { to }) => {
                if to.is_directory() {
                    let mut v = fc.read_dirs.lock();
                    v.insert(Bytes::from(path.clone()));
                }
                to.find_nodes(fc, results, path, d)
            }
            Some(NodeType::UnsupportedSymlink {}) => {
                log!("FindEmulator does not support symlink {path:?}");
                false
            }
            _ => true,
        }
    }
    fn run_find(
        self: &Arc<Self>,
        fc: &FindCommand,
        loc: &Loc,
        d: i32,
        path: &mut Vec<u8>,
        cur_read_dirs: &Arc<Mutex<HashMap<DirentNodeKey, Vec<u8>>>>,
        out: &mut Vec<Vec<u8>>,
    ) -> Result<bool> {
        match self.inner(fc.follows_symlink) {
            Some(NodeType::File { typ }) => {
                self.print_if_necessary(fc, path, *typ, d, out);
                Ok(true)
            }
            Some(NodeType::Dir { children, .. }) => {
                let srdt = ScopedReadDirTracker::new(self, path, cur_read_dirs);
                if let Some(conflicted) = &srdt.conflicted {
                    find_warn_loc!(
                        Some(loc),
                        "FindEmulator: find: File system loop detected; {:?} is part of the same file system loop as {:?}.",
                        String::from_utf8_lossy(path),
                        String::from_utf8_lossy(conflicted),
                    );
                    return Ok(true);
                }

                fc.read_dirs.lock().insert(Bytes::from(path.clone()));

                if fc
                    .prune_cond
                    .as_ref()
                    .is_some_and(|cond| cond.is_true(path, FileType::Dir))
                {
                    if fc.typ != Some(FindCommandType::FindLeaves) {
                        out.push(path.clone())
                    }
                    return Ok(true);
                }

                self.print_if_necessary(fc, path, FileType::Dir, d, out);

                if d >= fc.depth {
                    return Ok(true);
                }

                let orig_path_size = path.len();
                if fc.typ == Some(FindCommandType::FindLeaves) {
                    let orig_out_size = out.len();
                    for (_name, c) in children {
                        // We will handle directories later.
                        if c.is_directory() {
                            continue;
                        }
                        if !path.ends_with(b"/") {
                            path.push(b'/');
                        }
                        path.extend_from_slice(c.base.as_bytes());
                        if !c.run_find(fc, loc, d + 1, path, cur_read_dirs, out)? {
                            return Ok(false);
                        }
                        path.truncate(orig_path_size);
                    }

                    // Found a leaf, stop the search.
                    if orig_out_size != out.len() {
                        // If we've found all possible files in this directory, we don't need
                        // to add a regen dependency on the directory, we just need to ensure
                        // that the files are not removed.
                        let print_cond = fc.print_cond.as_ref().unwrap();
                        if print_cond.countable() && print_cond.count() == out.len() - orig_out_size
                        {
                            fc.read_dirs.lock().remove(&Bytes::from(path.clone()));
                            let mut i = orig_out_size;
                            let mut v = fc.found_files.lock();
                            while i < out.len() {
                                v.push(Bytes::from(out[i].clone()));
                                i += 1;
                            }
                        }

                        return Ok(true);
                    }

                    for (_name, c) in children {
                        if !c.is_directory() {
                            continue;
                        }
                        if !path.ends_with(b"/") {
                            path.push(b'/');
                        }
                        path.extend_from_slice(c.base.as_bytes());
                        if !c.run_find(fc, loc, d + 1, path, cur_read_dirs, out)? {
                            return Ok(false);
                        }
                        path.truncate(orig_path_size);
                    }
                } else {
                    for (_name, c) in children {
                        if !path.ends_with(b"/") {
                            path.push(b'/');
                        }
                        path.extend_from_slice(c.base.as_bytes());
                        if !c.run_find(fc, loc, d + 1, path, cur_read_dirs, out)? {
                            return Ok(false);
                        }
                        path.truncate(orig_path_size);
                    }
                }
                Ok(true)
            }
            Some(NodeType::Symlink { to }) => to.run_find(fc, loc, d, path, cur_read_dirs, out),
            Some(NodeType::SymlinkError { err }) => {
                if err.kind() == std::io::ErrorKind::NotFound {
                    self.print_if_necessary(fc, path, FileType::Symlink, d, out);
                    return Ok(true);
                }

                if fc.typ != Some(FindCommandType::FindLeaves) {
                    find_warn_loc!(
                        Some(loc),
                        "FindEmulator: find: {:?}: {err}",
                        String::from_utf8_lossy(path),
                    );
                    return Ok(true);
                }
                Ok(true)
            }
            Some(NodeType::UnsupportedSymlink {}) => {
                log!(
                    "FindEmulator does not support {}",
                    String::from_utf8_lossy(path)
                );
                Ok(false)
            }
            None => {
                self.print_if_necessary(fc, path, FileType::Symlink, d, out);
                Ok(true)
            }
            _ => Ok(true),
        }
    }

    fn inner(self: &Arc<Self>, follows_symlinks: bool) -> Option<&NodeType> {
        if !follows_symlinks {
            if let Some(inner) = self.inner.get() {
                match inner {
                    NodeType::Symlink { .. }
                    | NodeType::SymlinkError { .. }
                    | NodeType::UnsupportedSymlink { .. } => return None,
                    _ => {}
                }
            } else {
                let init_data = self.init_data.lock();
                if let Some(NodeTypeInitData::Symlink { .. }) = init_data.as_ref() {
                    return None;
                }
            }
        }
        Some(self.inner.get_or_init(|| {
            let mut init_data = self.init_data.lock();
            let Some(init_data) = init_data.take() else {
                return NodeType::Error {};
            };
            match init_data {
                NodeTypeInitData::Dir { name, parent } => self.initialize_dir(name, parent),
                NodeTypeInitData::Symlink { name, parent } => {
                    Self::initialize_symlink(name, parent)
                }
            }
        }))
    }
    fn initialize_symlink(name: PathBuf, parent: Weak<DirentNode>) -> NodeType {
        collect_stats!("init find emulator Dirent NodeType::Symlink initialize");
        let dest = match std::fs::read_link(&name) {
            Err(err) => {
                warn!("readlink failed: {:?}", err);
                return NodeType::SymlinkError { err };
            }
            Ok(path) => path,
        };

        if let Err(err) = std::fs::metadata(&name) {
            log!("stat failed: {:?}: {:?}", name, err);
            return NodeType::SymlinkError { err };
        }

        // absolute symlinks aren't supported by the find emulator
        if dest.is_absolute() {
            return NodeType::UnsupportedSymlink {};
        }

        let Some(parent) = parent.upgrade() else {
            return NodeType::SymlinkError {
                err: std::io::Error::from(std::io::ErrorKind::Other),
            };
        };

        let Some(to) = parent.find_dir(dest.as_os_str().as_bytes()) else {
            return NodeType::SymlinkError {
                err: std::io::Error::from(std::io::ErrorKind::Other),
            };
        };
        NodeType::Symlink { to }
    }

    fn initialize_dir(
        self: &Arc<Self>,
        name: PathBuf,
        parent: Option<Weak<DirentNode>>,
    ) -> NodeType {
        collect_stats!("init find emulator Dirent NodeType::Dir initialize");

        let entries = match std::fs::read_dir(&name) {
            Ok(entries) => entries,
            Err(err) => {
                warn!("opendir({:?}) failed: {:?}", name, err);
                return NodeType::Dir {
                    parent,
                    children: Vec::new(),
                };
            }
        };

        let mut children = Vec::new();
        for entry in entries {
            let entry = match entry {
                Ok(entry) => entry,
                Err(err) => {
                    warn!("readdir failed: {:?}", err);
                    continue;
                }
            };
            if entry.file_name() == "."
                || entry.file_name() == ".."
                || entry.file_name() == ".repo"
                || entry.file_name() == ".git"
            {
                continue;
            }

            let Some(typ) = entry.file_type().ok() else {
                warn!("stat failed: {:?}", entry.path());
                continue;
            };
            let path = entry.path();
            let base = OsStr::from_bytes(basename(path.as_os_str().as_bytes())).to_os_string();
            children.push((
                base.clone(),
                Arc::new(if typ.is_dir() {
                    DirentNode {
                        base,
                        inner: OnceLock::new(),
                        init_data: Mutex::new(Some(NodeTypeInitData::Dir {
                            name: path,
                            parent: Some(Arc::downgrade(self)),
                        })),
                    }
                } else if typ.is_symlink() {
                    DirentNode {
                        base,
                        inner: OnceLock::new(),
                        init_data: Mutex::new(Some(NodeTypeInitData::Symlink {
                            name: path,
                            parent: Arc::downgrade(self),
                        })),
                    }
                } else {
                    let inner = OnceLock::new();
                    let _ = inner.set(NodeType::File {
                        typ: typ.try_into().unwrap(),
                    });
                    DirentNode {
                        base,
                        inner,
                        init_data: Mutex::new(None),
                    }
                }),
            ))
        }
        NODE_COUNT.fetch_add(children.len(), std::sync::atomic::Ordering::Relaxed);

        NodeType::Dir { parent, children }
    }
}

struct DirentNodeKey(Arc<DirentNode>);

impl std::hash::Hash for DirentNodeKey {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        Arc::as_ptr(&self.0).hash(state)
    }
}
impl PartialEq for DirentNodeKey {
    fn eq(&self, other: &Self) -> bool {
        Arc::ptr_eq(&self.0, &other.0)
    }
}
impl Eq for DirentNodeKey {}

struct ScopedReadDirTracker {
    conflicted: Option<Vec<u8>>,
    n: Option<Arc<DirentNode>>,
    cur_read_dirs: Arc<Mutex<HashMap<DirentNodeKey, Vec<u8>>>>,
}

impl ScopedReadDirTracker {
    fn new(
        node: &Arc<DirentNode>,
        path: &[u8],
        cur_read_dirs: &Arc<Mutex<HashMap<DirentNodeKey, Vec<u8>>>>,
    ) -> Self {
        let mut conflicted = None;
        let mut n = None;
        let key = DirentNodeKey(node.clone());
        {
            let mut dirs = cur_read_dirs.lock();
            if let Some(old) = dirs.get(&key) {
                conflicted = Some(old.clone());
            } else {
                dirs.insert(key, path.to_vec());
                n = Some(node.clone());
            }
        }
        Self {
            conflicted,
            n,
            cur_read_dirs: cur_read_dirs.clone(),
        }
    }
}

impl Drop for ScopedReadDirTracker {
    fn drop(&mut self) {
        if let Some(n) = &self.n {
            self.cur_read_dirs.lock().remove(&DirentNodeKey(n.clone()));
        }
    }
}

struct FindCommandParser {
    cmd: Bytes,
    cur: Bytes,
    fc: FindCommand,
    has_if: bool,
    unget_tok: Option<Bytes>,
}

impl FindCommandParser {
    fn new(cmd: &Bytes) -> Self {
        FindCommandParser {
            cmd: cmd.clone(),
            cur: cmd.clone(),
            fc: FindCommand::default(),
            has_if: false,
            unget_tok: None,
        }
    }

    fn parse(mut self) -> Result<Option<FindCommand>> {
        self.cur = self.cmd.clone();
        if !self.parse_impl()? {
            log!(
                "*kati*: FindEmulator: Unsupported find command: {:?}",
                String::from_utf8_lossy(&self.cmd)
            );
            return Ok(None);
        }
        assert!(trim_left_space(&self.cur).is_empty());
        Ok(Some(self.fc))
    }

    fn get_next_token(&mut self) -> Option<Bytes> {
        if self.unget_tok.is_some() {
            return std::mem::take(&mut self.unget_tok);
        }

        self.cur = self.cur.slice_ref(trim_left_space(&self.cur));

        if self.cur.starts_with(b";") {
            let tok = self.cur.slice(..1);
            self.cur.advance(1);
            return Some(tok);
        }
        if self.cur.starts_with(b"&&") {
            let tok = self.cur.slice(0..2);
            self.cur.advance(2);
            return Some(tok);
        }
        if self.cur.starts_with(b"&") {
            return None;
        }

        let mut i = 0;
        while i < self.cur.len() {
            let c = self.cur[i];
            if c.is_ascii_whitespace() || c == b';' || c == b'&' {
                break;
            }
            i += 1
        }

        let mut tok = self.cur.slice(..i);
        self.cur.advance(i);

        if tok.is_empty() {
            return Some(tok);
        }
        let c = tok[0];
        if c == b'\'' || c == b'"' {
            if tok.len() < 2 || tok.last() != Some(&c) {
                return None;
            }
            tok = tok.slice(1..tok.len() - 1);
        } else {
            // Support stripping off a leading backslash
            if c == b'\\' {
                tok.advance(1);
            }
            // But if there are any others, we can't support it, as unescaping would
            // require allocation
            if memchr(b'\\', &tok).is_some() {
                return None;
            }
        }

        Some(tok)
    }

    fn unget_token(&mut self, tok: Bytes) {
        assert!(self.unget_tok.is_none());
        if !tok.is_empty() {
            self.unget_tok = Some(tok)
        }
    }

    fn parse_test(&mut self) -> bool {
        if self.has_if || self.fc.testdir.is_some() {
            return false;
        }
        if self.get_next_token().is_none_or(|t| t.as_ref() != b"-d") {
            return false;
        }
        let Some(tok) = self.get_next_token() else {
            return false;
        };
        if tok.is_empty() {
            return false;
        }
        self.fc.testdir = Some(tok);
        true
    }

    fn parse_fact(&mut self, tok: Bytes) -> Option<FindCond> {
        match tok.as_ref() {
            b"-not" | b"!" => {
                let tok = self.get_next_token()?;
                if tok.is_empty() {
                    return None;
                }
                let c = self.parse_fact(tok)?;
                Some(FindCond::new_not(c))
            }
            b"(" => {
                let tok = self.get_next_token()?;
                if tok.is_empty() {
                    return None;
                }
                let c = self.parse_expr(tok);
                let tok = self.get_next_token()?;
                if tok.is_empty() {
                    return None;
                }
                c
            }
            b"-name" => {
                let tok = self.get_next_token()?;
                if tok.is_empty() {
                    return None;
                }
                FindCond::new_name(&tok).ok()
            }
            b"-type" => {
                let tok = self.get_next_token()?;
                if tok.is_empty() {
                    return None;
                }
                let typ = match tok.as_ref() {
                    b"b" => FileType::BlockDevice,
                    b"c" => FileType::CharDevice,
                    b"d" => FileType::Dir,
                    b"p" => FileType::Fifo,
                    b"l" => FileType::Symlink,
                    b"f" => FileType::Regular,
                    b"s" => FileType::Socket,
                    _ => return None,
                };
                Some(FindCond::new_type(typ))
            }
            _ => {
                self.unget_token(tok);
                None
            }
        }
    }

    fn parse_term(&mut self, tok: Bytes) -> Option<FindCond> {
        let mut c = self.parse_fact(tok)?;
        loop {
            let mut tok = self.get_next_token()?;
            if tok.as_ref() == b"-and" || tok.as_ref() == b"-a" {
                tok = self.get_next_token()?;
                if tok.is_empty() {
                    return None;
                }
            } else if tok.as_ref() != b"-not"
                && tok.as_ref() != b"!"
                && tok.as_ref() != b"("
                && tok.as_ref() != b"-name"
                && tok.as_ref() != b"-type"
            {
                self.unget_token(tok);
                return Some(c);
            }
            let r = self.parse_fact(tok)?;
            c = FindCond::new_and(c, r);
        }
    }

    fn parse_expr(&mut self, tok: Bytes) -> Option<FindCond> {
        let mut c = self.parse_term(tok)?;
        loop {
            let tok = self.get_next_token()?;
            if tok != "-or" && tok != "-o" {
                self.unget_token(tok);
                return Some(c);
            }
            let tok = self.get_next_token()?;
            if tok.is_empty() {
                return None;
            }
            let r = self.parse_term(tok)?;
            c = FindCond::new_or(c, r);
        }
    }

    // <expr> ::= <term> {<or> <term>}
    // <term> ::= <fact> {[<and>] <fact>}
    // <fact> ::= <not> <fact> | '(' <expr> ')' | <pred>
    // <not> ::= '-not' | '!'
    // <and> ::= '-and' | '-a'
    // <or> ::= '-or' | '-o'
    // <pred> ::= <name> | <type> | <maxdepth>
    // <name> ::= '-name' NAME
    // <type> ::= '-type' TYPE
    // <maxdepth> ::= '-maxdepth' MAXDEPTH
    fn parse_find_cond(&mut self, tok: Bytes) -> Option<FindCond> {
        self.parse_expr(tok)
    }

    fn parse_find(&mut self) -> bool {
        self.fc.typ = Some(FindCommandType::Find);
        loop {
            let Some(tok) = self.get_next_token() else {
                return false;
            };
            if tok.is_empty() || tok == ";" {
                return true;
            }

            if tok == "-L" {
                self.fc.follows_symlink = true;
            } else if tok == "-prune" {
                if self.fc.print_cond.is_none() || self.fc.prune_cond.is_some() {
                    return false;
                }
                if self.get_next_token().is_none_or(|t| t != "-o") {
                    return false;
                }
                self.fc.prune_cond = std::mem::take(&mut self.fc.print_cond);
            } else if tok == "-print" {
                let Some(tok) = self.get_next_token() else {
                    return false;
                };
                if !tok.is_empty() {
                    return false;
                }
                return true;
            } else if tok == "-maxdepth" {
                let Some(tok) = self.get_next_token() else {
                    return false;
                };
                if tok.is_empty() {
                    return false;
                }
                let Ok(depth) = String::from_utf8_lossy(&tok).parse::<i32>() else {
                    return false;
                };
                if depth < 0 {
                    return false;
                }
                self.fc.depth = depth;
            } else if tok.starts_with(b"-") || tok.as_ref() == b"(" || tok.as_ref() == b"!" {
                if self.fc.print_cond.is_some() {
                    return false;
                }
                let Some(c) = self.parse_find_cond(tok) else {
                    return false;
                };
                self.fc.print_cond = Some(c);
            } else if tok == "2>" {
                if self
                    .get_next_token()
                    .is_none_or(|t| t.as_ref() != b"/dev/null")
                {
                    return false;
                }
                self.fc.redirect_to_devnull = true;
            } else if tok
                .iter()
                .any(|c| [b'|', b';', b'&', b'>', b'<', b'\'', b'"'].contains(c))
            {
                return false;
            } else {
                self.fc.finddirs.push(tok);
            }
        }
    }

    fn parse_find_leaves(&mut self) -> Result<bool> {
        self.fc.typ = Some(FindCommandType::FindLeaves);
        self.fc.follows_symlink = true;
        let mut findfiles = Vec::new();
        loop {
            let Some(tok) = self.get_next_token() else {
                return Ok(false);
            };
            if tok.is_empty() {
                if self.fc.finddirs.is_empty() {
                    // backwards compatibility
                    if findfiles.len() < 2 {
                        return Ok(false);
                    }
                    std::mem::swap(&mut self.fc.finddirs, &mut findfiles);
                    let Ok(cond) = FindCond::new_name(&self.fc.finddirs.pop().unwrap()) else {
                        return Ok(false);
                    };
                    self.fc.print_cond = Some(cond);
                } else {
                    if findfiles.is_empty() {
                        return Ok(false);
                    }
                    for file in findfiles {
                        let Ok(mut cond) = FindCond::new_name(&file) else {
                            return Ok(false);
                        };
                        if self.fc.print_cond.is_some() {
                            cond = FindCond::new_or(
                                std::mem::take(&mut self.fc.print_cond).unwrap(),
                                cond,
                            )
                        }
                        assert!(self.fc.print_cond.is_none());
                        self.fc.print_cond = Some(cond);
                    }
                }
                return Ok(true);
            }

            if let Some(prune) = tok.strip_prefix(b"--prune=") {
                let Ok(mut cond) = FindCond::new_name(prune) else {
                    return Ok(false);
                };
                if self.fc.prune_cond.is_some() {
                    cond = FindCond::new_or(std::mem::take(&mut self.fc.prune_cond).unwrap(), cond)
                }
                assert!(self.fc.prune_cond.is_none());
                self.fc.prune_cond = Some(cond);
            } else if let Some(mindepth) = tok.strip_prefix(b"--mindepth=") {
                let Ok(mindepth) = String::from_utf8_lossy(mindepth).parse::<i32>() else {
                    return Ok(false);
                };
                self.fc.mindepth = mindepth;
            } else if let Some(dir) = tok.strip_prefix(b"--dir=") {
                self.fc.finddirs.push(Bytes::from(dir.to_vec()));
            } else if tok.starts_with(b"--") {
                if FLAGS.werror_find_emulator {
                    error!(
                        "Unknown flag in findleaves.py: {}",
                        String::from_utf8_lossy(&tok)
                    );
                } else {
                    warn!(
                        "Unknown flag in findleaves.py: {}",
                        String::from_utf8_lossy(&tok)
                    );
                }
                return Ok(false);
            } else {
                findfiles.push(tok);
            }
        }
    }

    fn parse_impl(&mut self) -> Result<bool> {
        loop {
            let Some(tok) = self.get_next_token() else {
                return Ok(false);
            };

            if tok.is_empty() {
                return Ok(true);
            }

            if tok.as_ref() == b"cd" {
                let Some(tok) = self.get_next_token() else {
                    return Ok(false);
                };
                if tok.is_empty() || self.fc.chdir.is_some() {
                    return Ok(false);
                }
                if memchr3(b'?', b'*', b'[', &tok).is_some() {
                    return Ok(false);
                }
                self.fc.chdir = Some(tok);
                let Some(tok) = self.get_next_token() else {
                    return Ok(false);
                };
                if tok != ";" && tok != "&&" {
                    return Ok(false);
                }
            } else if tok.as_ref() == b"if" {
                if self.get_next_token().is_none_or(|t| t.as_ref() != b"[") {
                    return Ok(false);
                }
                if !self.parse_test() {
                    return Ok(false);
                }
                if self.get_next_token().is_none_or(|t| t.as_ref() != b"]") {
                    return Ok(false);
                }
                if self.get_next_token().is_none_or(|t| t.as_ref() != b";") {
                    return Ok(false);
                }
                if self.get_next_token().is_none_or(|t| t.as_ref() != b"then") {
                    return Ok(false);
                }
                self.has_if = true
            } else if tok.as_ref() == b"test" {
                if self.fc.chdir.is_some() {
                    return Ok(false);
                }
                if !self.parse_test() {
                    return Ok(false);
                }
                if self.get_next_token().is_none_or(|t| t.as_ref() != b"&&") {
                    return Ok(false);
                }
            } else if tok.as_ref() == b"find" {
                if !self.parse_find() {
                    return Ok(false);
                }
                if self.has_if && self.get_next_token().is_none_or(|t| t.as_ref() != b"fi") {
                    return Ok(false);
                }
                if self.get_next_token().is_none_or(|t| t.as_ref() != b"") {
                    return Ok(false);
                }
                return Ok(true);
            } else if tok.as_ref() == b"build/tools/findleaves.py"
                || tok.as_ref() == b"build/make/tools/findleaves.py"
            {
                return self.parse_find_leaves();
            } else {
                return Ok(false);
            }
        }
    }
}

pub struct FindEmulator {
    root: Arc<DirentNode>,
}

impl FindEmulator {
    fn new() -> Self {
        Self {
            root: DirentNode::new(),
        }
    }

    fn can_handle(s: &[u8]) -> bool {
        !s.starts_with(b"/") && !s.starts_with(b".repo") && !s.starts_with(b".git")
    }

    fn find_dir(&self, d: &[u8], should_fallback: &mut bool) -> Option<Arc<DirentNode>> {
        let r = self.root.find_dir(d);
        if r.is_none() {
            *should_fallback = std::fs::exists(OsStr::from_bytes(d)).unwrap_or(false);
        }
        r
    }

    fn handle_find(
        &self,
        cmd: &Bytes,
        fc: &FindCommand,
        loc: &Loc,
        out: &mut BytesMut,
    ) -> Result<bool> {
        if let Some(chdir) = &fc.chdir
            && !Self::can_handle(chdir)
        {
            log!("FindEmulator: Cannot handle chdir ({chdir:?}): {cmd:?}");
            return Ok(false);
        }

        if let Some(testdir) = &fc.testdir {
            if !Self::can_handle(testdir) {
                log!("FindEmulator: Cannot handle test dir ({testdir:?}): {cmd:?}");
                return Ok(false);
            }
            let mut should_fallback = false;
            if self.find_dir(testdir, &mut should_fallback).is_none() {
                log!("FindEmulator: Test dir ({testdir:?}) not found: {cmd:?}");
                return Ok(!should_fallback);
            }
        }

        let mut root = self.root.clone();

        let mut fc_chdir = Bytes::new();
        if let Some(chdir) = &fc.chdir {
            if !Self::can_handle(chdir) {
                log!("FindEmulator: Cannot handle chdir ({chdir:?}): {cmd:?}");
                return Ok(false);
            }
            let Some(new_root) = root.find_dir(chdir) else {
                if std::fs::exists(OsStr::from_bytes(chdir)).unwrap_or(false) {
                    return Ok(false);
                }
                if !fc.redirect_to_devnull {
                    find_warn_loc!(
                        Some(loc),
                        "FindEmulator: cd: {}: No such file or directory",
                        String::from_utf8_lossy(chdir)
                    );
                }
                return Ok(true);
            };
            root = new_root;
            fc_chdir = chdir.clone();
        }

        let mut results = Vec::new();
        for finddir in &fc.finddirs {
            let fullpath = concat_dir(&fc_chdir, finddir);
            if !Self::can_handle(&fullpath) {
                log!("FindEmulator: Cannot handle find dir ({fullpath:?}): {cmd:?}");
                return Ok(false);
            }

            let mut findnodestr = Vec::new();
            let mut bases = Vec::new();
            if !root.find_nodes(fc, &mut bases, &mut findnodestr, finddir) {
                return Ok(false);
            }
            if bases.is_empty() {
                if std::fs::exists(OsStr::from_bytes(&fullpath)).unwrap_or(false) {
                    return Ok(false);
                }
                if !fc.redirect_to_devnull {
                    find_warn_loc!(
                        Some(loc),
                        "FindEmulator: find: \"{}\": No such file or directory",
                        String::from_utf8_lossy(&fullpath)
                    );
                }
                continue;
            }

            // bash guarantees that globs are sorted
            bases.sort_by(|a, b| a.0.cmp(&b.0));

            for (mut path, base) in bases {
                let cur_read_dirs = Arc::new(Mutex::new(HashMap::new()));
                if !base.run_find(fc, loc, 0, &mut path, &cur_read_dirs, &mut results)? {
                    log!(
                        "FindEmulator: RunFind failed: {}",
                        String::from_utf8_lossy(cmd)
                    );
                    return Ok(false);
                }
            }
        }

        if !results.is_empty() {
            // Calculate and reserve necessary space in out
            let mut new_length = 0usize;
            for result in &results {
                new_length += result.len() + 1;
            }
            out.reserve(new_length - 1);

            if fc.typ == Some(FindCommandType::FindLeaves) {
                results.sort();
            }

            let mut writer = WordWriter::new(out);
            for result in results {
                writer.write(&result);
            }
        }

        log!("FindEmulator: OK");
        Ok(true)
    }
}

#[derive(Debug)]
pub struct FindCommand {
    typ: Option<FindCommandType>,
    pub chdir: Option<Bytes>,
    testdir: Option<Bytes>,
    pub finddirs: Vec<Bytes>,
    follows_symlink: bool,
    print_cond: Option<FindCond>,
    prune_cond: Option<FindCond>,
    depth: i32,
    mindepth: i32,
    redirect_to_devnull: bool,

    pub found_files: Mutex<Vec<Bytes>>,
    pub read_dirs: Mutex<HashSet<Bytes>>,
}

impl Default for FindCommand {
    fn default() -> Self {
        Self {
            typ: None,
            chdir: None,
            testdir: None,
            finddirs: Vec::new(),
            follows_symlink: false,
            print_cond: None,
            prune_cond: None,
            depth: i32::MAX,
            mindepth: i32::MIN,
            redirect_to_devnull: false,

            found_files: Mutex::new(Vec::new()),
            read_dirs: Mutex::new(HashSet::new()),
        }
    }
}

impl PartialEq for FindCommand {
    fn eq(&self, other: &Self) -> bool {
        // We ignore found_files/read_dirs, so we don't need to grab the mutex
        self.typ == other.typ
            && self.chdir == other.chdir
            && self.testdir == other.testdir
            && self.finddirs == other.finddirs
            && self.follows_symlink == other.follows_symlink
            && self.print_cond == other.print_cond
            && self.prune_cond == other.prune_cond
            && self.depth == other.depth
            && self.mindepth == other.mindepth
            && self.redirect_to_devnull == other.redirect_to_devnull
    }
}

impl FindCommand {
    fn with_cmd(cmd: &Bytes) -> Result<Option<Self>> {
        if !has_word(cmd, b"find")
            && !has_word(cmd, b"build/tools/findleaves.py")
            && !has_word(cmd, b"build/make/tools/findleaves.py")
        {
            return Ok(None);
        }

        let Some(mut fc) = FindCommandParser::new(cmd).parse()? else {
            return Ok(None);
        };

        if let Some(ref mut chdir) = fc.chdir {
            *chdir = normalize_path(chdir);
        }
        if let Some(ref mut testdir) = fc.testdir {
            *testdir = normalize_path(testdir);
        }

        if fc.finddirs.is_empty() {
            fc.finddirs.push(Bytes::from_static(b"."));
        }

        Ok(Some(fc))
    }
}

pub fn parse(cmd: &Bytes) -> Result<Option<FindCommand>> {
    FindCommand::with_cmd(cmd)
}

static FIND_EMULATOR: LazyLock<FindEmulator> = LazyLock::new(FindEmulator::new);

pub fn find(cmd: &Bytes, fc: &FindCommand, loc: &Loc) -> Result<Option<Bytes>> {
    let mut out = BytesMut::new();
    if !FIND_EMULATOR.handle_find(cmd, fc, loc, &mut out)? {
        return Ok(None);
    }
    Ok(Some(out.freeze()))
}

pub fn get_node_count() -> usize {
    NODE_COUNT.load(std::sync::atomic::Ordering::Relaxed)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_find() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(b"find ."))
            .unwrap()
            .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b".")],
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_find_follow_symlink() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(b"find -L ."))
            .unwrap()
            .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b".")],
                follows_symlink: true,
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_find_dirs() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(b"find top/C bar"))
            .unwrap()
            .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b"top/C"), Bytes::from_static(b"bar")],
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_cd_find() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(b"cd top && find C"))
            .unwrap()
            .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                chdir: Some(Bytes::from_static(b"top")),
                finddirs: vec![Bytes::from_static(b"C")],
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_find_conds() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(
            b"find top -type f -name 'a*' -o -name \\*b",
        ))
        .unwrap()
        .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b"top")],
                print_cond: Some(FindCond::new_or(
                    FindCond::new_and(
                        FindCond::new_type(FileType::Regular),
                        FindCond::new_name(b"a*").unwrap()
                    ),
                    FindCond::new_name(b"*b").unwrap()
                )),
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_find_conds_paren() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(
            b"find top -type f -a \\( -name 'a*' -o -name \\*b \\)",
        ))
        .unwrap()
        .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b"top")],
                print_cond: Some(FindCond::new_and(
                    FindCond::new_type(FileType::Regular),
                    FindCond::new_or(
                        FindCond::new_name(b"a*").unwrap(),
                        FindCond::new_name(b"*b").unwrap(),
                    )
                )),
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_find_not() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(b"find top \\! -name 'a*'"))
            .unwrap()
            .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b"top")],
                print_cond: Some(FindCond::new_not(FindCond::new_name(b"a*").unwrap())),
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_find_paren() {
        let fc = FindCommand::with_cmd(&Bytes::from_static(b"find top \\( -name 'a*' \\)"))
            .unwrap()
            .unwrap();
        assert_eq!(
            fc,
            FindCommand {
                typ: Some(FindCommandType::Find),
                finddirs: vec![Bytes::from_static(b"top")],
                print_cond: Some(FindCond::new_name(b"a*").unwrap()),
                ..FindCommand::default()
            }
        );
    }

    #[test]
    fn test_parse_fail() {
        assert_eq!(
            FindCommand::with_cmd(&Bytes::from_static(b"find top -name a\\*")).unwrap(),
            None
        );
        // * in a chdir is not supported
        assert_eq!(
            FindCommand::with_cmd(&Bytes::from_static(b"cd top/*/B && find .")).unwrap(),
            None
        );
    }
}
