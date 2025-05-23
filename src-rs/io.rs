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
    ffi::OsString,
    io::Write,
    os::unix::ffi::OsStringExt,
    time::{Duration, UNIX_EPOCH},
};

use anyhow::Result;

pub fn dump_int(out: &mut impl Write, val: i32) -> Result<()> {
    out.write_all(&val.to_le_bytes())?;
    Ok(())
}

pub fn load_int(f: &mut impl std::io::Read) -> Option<i32> {
    let mut buf = [0u8; 4];
    f.read_exact(&mut buf).ok()?;
    Some(i32::from_le_bytes(buf))
}

pub fn dump_usize(out: &mut impl Write, val: usize) -> Result<()> {
    dump_int(out, val as i32)
}

pub fn load_usize(f: &mut impl std::io::Read) -> Option<usize> {
    let v = load_int(f)?;
    if v < 0 { None } else { Some(v as usize) }
}

pub fn dump_string(out: &mut impl Write, s: &[u8]) -> Result<()> {
    dump_usize(out, s.len())?;
    out.write_all(s)?;
    Ok(())
}

pub fn load_string(f: &mut impl std::io::Read) -> Option<OsString> {
    let len = load_usize(f)?;
    let mut v = vec![0; len];
    f.read_exact(&mut v[..]).ok()?;
    Some(OsString::from_vec(v))
}

pub fn dump_vec_string<T: AsRef<[u8]>>(out: &mut impl Write, v: &[T]) -> Result<()> {
    dump_usize(out, v.len())?;
    for s in v {
        dump_string(out, s.as_ref())?;
    }
    Ok(())
}

pub fn load_vec_string(f: &mut impl std::io::Read) -> Option<Vec<OsString>> {
    let len = load_usize(f)?;
    let mut ret = Vec::with_capacity(len);
    for _ in 0..len {
        ret.push(load_string(f)?);
    }
    Some(ret)
}

pub fn dump_systemtime(out: &mut impl Write, t: &std::time::SystemTime) -> Result<()> {
    let dur = t.duration_since(UNIX_EPOCH)?;
    out.write_all(&dur.as_secs_f64().to_le_bytes())?;
    Ok(())
}

pub fn load_systemtime(f: &mut impl std::io::Read) -> Option<std::time::SystemTime> {
    let mut buf = [0u8; 8];
    f.read_exact(&mut buf).ok()?;
    Some(UNIX_EPOCH + Duration::from_secs_f64(f64::from_le_bytes(buf)))
}

#[cfg(test)]
mod tests {
    use std::os::unix::ffi::OsStrExt;

    use super::*;

    #[test]
    fn test_int() {
        let mut buf = Vec::new();
        dump_int(&mut buf, 12345).unwrap();
        let mut buf = &buf[..];
        assert_eq!(load_int(&mut buf), Some(12345));
    }

    #[test]
    fn test_string() {
        let s = OsString::from("Hello World!");
        let mut buf = Vec::new();
        dump_string(&mut buf, s.as_bytes()).unwrap();
        let mut buf = &buf[..];
        assert_eq!(load_string(&mut buf), Some(s));
    }

    #[test]
    fn test_empty_string() {
        let s = OsString::new();
        let mut buf = Vec::new();
        dump_string(&mut buf, s.as_bytes()).unwrap();
        let mut buf = &buf[..];
        assert_eq!(load_string(&mut buf), Some(s));
    }
}
