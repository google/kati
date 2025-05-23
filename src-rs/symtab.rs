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
    fmt::{Debug, Display},
    num::NonZeroUsize,
    sync::LazyLock,
    vec,
};

use crate::{
    error,
    var::{Var, VarOrigin, Variable},
};
use anyhow::Result;
use bytes::{BufMut, Bytes, BytesMut};
use parking_lot::Mutex;

static SYMTAB: LazyLock<Mutex<Symtab>> = LazyLock::new(|| Mutex::new(Symtab::new()));

pub static SHELL_SYM: LazyLock<Symbol> = LazyLock::new(|| intern("SHELL"));
pub static ALLOW_RULES_SYM: LazyLock<Symbol> = LazyLock::new(|| intern(".KATI_ALLOW_RULES"));
pub static KATI_READONLY_SYM: LazyLock<Symbol> = LazyLock::new(|| intern(".KATI_READONLY"));
pub static VARIABLES_SYM: LazyLock<Symbol> = LazyLock::new(|| intern(".VARIABLES"));
pub static KATI_SYMBOLS_SYM: LazyLock<Symbol> = LazyLock::new(|| intern(".KATI_SYMBOLS"));
pub static MAKEFILE_LIST: LazyLock<Symbol> = LazyLock::new(|| intern("MAKEFILE_LIST"));

#[derive(Clone, Copy, PartialEq, Eq, Hash)]
pub struct Symbol(NonZeroUsize);

impl Display for Symbol {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let r = SYMTAB.lock();
        write!(f, "{}", String::from_utf8_lossy(&r.symbols[self.0.get()]))
    }
}

impl Debug for Symbol {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let r = SYMTAB.lock();
        write!(f, "{:?}({})", r.symbols[self.0.get()], self.0.get())
    }
}

impl Symbol {
    pub fn as_bytes(&self) -> Bytes {
        let r = SYMTAB.lock();
        r.symbols[self.0.get()].clone()
    }

    pub fn peek_global_var(&self) -> Option<Var> {
        let r = SYMTAB.lock();
        r.symbol_data.get(self.0.get())?.clone()
    }

    pub fn get_global_var(&self) -> Option<Var> {
        let v = {
            let r = SYMTAB.lock();
            r.symbol_data.get(self.0.get())?.clone()?
        };
        match v.read().origin() {
            VarOrigin::Environment | VarOrigin::EnvironmentOverride => {
                crate::var::USED_ENV_VARS.lock().insert(*self);
            }
            _ => {}
        }
        Some(v)
    }

    pub fn set_global_var(
        &self,
        var: Var,
        is_override: bool,
        readonly: Option<&mut bool>,
    ) -> Result<()> {
        let mut r = SYMTAB.lock();
        r.set_global_var(self, var, is_override, readonly)
    }
}

pub struct ScopedGlobalVar {
    sym: Symbol,
    orig: Option<Var>,
}

impl ScopedGlobalVar {
    pub fn new(sym: Symbol, var: Var) -> Result<Self> {
        let orig = sym.peek_global_var();
        let mut symtab = SYMTAB.lock();
        let idx = sym.0.get();
        if idx >= symtab.symbol_data.len() {
            symtab.symbol_data.resize(idx + 1, None);
        }
        symtab.symbol_data[idx] = Some(var);
        Ok(Self { sym, orig })
    }
}

impl Drop for ScopedGlobalVar {
    fn drop(&mut self) {
        let mut r = SYMTAB.lock();
        let idx = self.sym.0.get();
        r.symbol_data[idx] = self.orig.clone();
    }
}

struct Symtab {
    symbols: Vec<Bytes>,
    symbol_data: Vec<Option<Var>>,
    symtab: HashMap<Bytes, Symbol>,
}

impl Symtab {
    fn new() -> Self {
        let mut symtab = Self {
            symbols: vec![Bytes::new()],
            symbol_data: vec![],
            symtab: HashMap::new(),
        };
        for i in 1u8..=255 {
            assert!(symtab.symbols.len() == i as usize);
            let name = Bytes::from(vec![i]);
            let sym = Symbol(NonZeroUsize::new(i.into()).unwrap());
            symtab.symbols.push(name.clone());
            symtab.symtab.insert(name, sym);
        }

        let shell_status_sym = symtab.intern(".SHELLSTATUS");
        symtab
            .set_global_var(
                &shell_status_sym,
                Variable::new_shell_status_var(),
                false,
                None,
            )
            .unwrap();
        let variables_sym = symtab.intern(".VARIABLES");
        symtab
            .set_global_var(
                &variables_sym,
                Variable::new_variable_names(b".VARIABLES", true),
                false,
                None,
            )
            .unwrap();
        let symbols_sym = symtab.intern(".KATI_SYMBOLS");
        symtab
            .set_global_var(
                &symbols_sym,
                Variable::new_variable_names(b".KATI_SYMBOLS", false),
                false,
                None,
            )
            .unwrap();

        symtab
    }

    fn intern<T: Into<Bytes> + AsRef<[u8]>>(&mut self, s: T) -> Symbol {
        if let [c] = s.as_ref() {
            return Symbol(NonZeroUsize::new(*c as usize).unwrap());
        }
        let s = s.into();
        if let Some(sym) = self.symtab.get(&s) {
            return *sym;
        }
        let sym = Symbol(NonZeroUsize::new(self.symbols.len()).unwrap());
        self.symbols.push(s.clone());
        self.symtab.insert(s, sym);
        sym
    }

    fn set_global_var(
        &mut self,
        sym: &Symbol,
        var: Var,
        is_override: bool,
        readonly: Option<&mut bool>,
    ) -> Result<()> {
        let idx = sym.0.get();
        if idx >= self.symbol_data.len() {
            self.symbol_data.resize(idx + 1, None);
        }
        let entry = self.symbol_data.get_mut(idx).unwrap();
        if let Some(orig) = entry {
            if orig.read().readonly {
                if let Some(readonly) = readonly {
                    *readonly = true;
                } else {
                    error!("*** cannot assign to readonly variable: {sym}");
                }
                return Ok(());
            } else if let Some(readonly) = readonly {
                *readonly = false;
            }
            let origin = orig.read().origin();
            if !is_override
                && (origin == VarOrigin::Override || origin == VarOrigin::EnvironmentOverride)
            {
                return Ok(());
            }
            if origin == VarOrigin::CommandLine && var.read().origin() == VarOrigin::File {
                return Ok(());
            }
            if origin == VarOrigin::Automatic {
                error!("overriding automatic variable is not implemented yet");
            }
        }
        *entry = Some(var);
        Ok(())
    }
}

pub fn intern<T: Into<Bytes> + AsRef<[u8]>>(s: T) -> Symbol {
    let mut w = SYMTAB.lock();
    w.intern(s)
}

pub fn join_symbols(symbols: &[Symbol], sep: &[u8]) -> Bytes {
    let mut r = BytesMut::new();
    let mut first = true;
    for s in symbols {
        if !first {
            r.put_slice(sep);
        } else {
            first = false;
        }
        r.put_slice(&s.as_bytes());
    }
    r.freeze()
}

pub fn get_symbol_names<T: Fn(Var) -> bool>(filter: T) -> Vec<(Symbol, Bytes)> {
    let s = SYMTAB.lock();
    s.symbols
        .iter()
        .enumerate()
        .filter_map(|(idx, str)| {
            let var = s.symbol_data.get(idx)?.clone()?;
            if !filter(var) {
                return None;
            }
            Some((Symbol(NonZeroUsize::new(idx).unwrap()), str.clone()))
        })
        .collect::<Vec<_>>()
}

pub fn symbol_count() -> usize {
    let s = SYMTAB.lock();
    s.symbols.len()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_intern() {
        let sym = intern("foo");
        let sym2 = intern("bar");
        let sym3 = intern("foo");
        assert_ne!(sym, sym2);
        assert_eq!(sym, sym3);
    }

    #[test]
    fn test_symbol_to_string() {
        let sym = intern("foo");
        assert_eq!(sym.to_string(), "foo");
    }

    #[test]
    fn test_single_letter_symbol() {
        let sym = intern("a");
        assert_eq!(sym.0.get(), 'a' as usize);
    }
}
