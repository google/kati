// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kati

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/golang/glog"
)

type fileid struct {
	dev, ino uint64
}

var (
	unknownFileid = fileid{}
	invalidFileid = fileid{dev: 1<<64 - 1, ino: 1<<64 - 1}
)

type dirent struct {
	id    fileid
	name  string
	lmode os.FileMode
	mode  os.FileMode
	// add other fields to support more find commands?
}

type fsCacheT struct {
	mu      sync.Mutex
	ids     map[string]fileid
	dirents map[fileid][]dirent
}

var fsCache = &fsCacheT{
	ids: make(map[string]fileid),
	dirents: map[fileid][]dirent{
		invalidFileid: nil,
	},
}

func init() {
	fsCache.readdir(".", unknownFileid)
}

func (c *fsCacheT) dirs() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.dirents)
}

func (c *fsCacheT) files() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, ents := range c.dirents {
		n += len(ents)
	}
	return n
}

func hasWildcardMeta(pat string) bool {
	return strings.IndexAny(pat, "*?[") >= 0
}

func hasWildcardMetaByte(pat []byte) bool {
	return bytes.IndexAny(pat, "*?[") >= 0
}

func wildcardUnescape(pat string) string {
	var buf bytes.Buffer
	for i := 0; i < len(pat); i++ {
		if pat[i] == '\\' && i+1 < len(pat) {
			switch pat[i+1] {
			case '*', '?', '[', '\\':
				buf.WriteByte(pat[i])
			}
			continue
		}
		buf.WriteByte(pat[i])
	}
	return buf.String()
}

func filepathJoin(names ...string) string {
	var dir string
	for i, n := range names {
		dir += n
		if i != len(names)-1 && n != "" && n[len(n)-1] != '/' {
			dir += "/"
		}
	}
	return dir
}

func filepathClean(path string) string {
	var names []string
	if filepath.IsAbs(path) {
		names = append(names, "")
	}
	paths := strings.Split(path, string(filepath.Separator))
Loop:
	for _, n := range paths {
		if n == "" || n == "." {
			continue Loop
		}
		if n == ".." && len(names) > 0 {
			dir, last := names[:len(names)-1], names[len(names)-1]
			parent := strings.Join(dir, string(filepath.Separator))
			if parent == "" {
				parent = "."
			}
			_, ents := fsCache.readdir(parent, unknownFileid)
			for _, e := range ents {
				if e.name != last {
					continue
				}
				if e.lmode&os.ModeSymlink == os.ModeSymlink && e.mode&os.ModeDir == os.ModeDir {
					// preserve .. if last is symlink dir.
					names = append(names, "..")
					continue Loop
				}
				// last is not symlink, maybe safe to clean.
				names = names[:len(names)-1]
				continue Loop
			}
			// parent doesn't exists? preserve ..
			names = append(names, "..")
			continue Loop
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return "."
	}
	return strings.Join(names, string(filepath.Separator))
}

func (c *fsCacheT) fileid(dir string) fileid {
	c.mu.Lock()
	id := c.ids[dir]
	c.mu.Unlock()
	return id
}

func (c *fsCacheT) readdir(dir string, id fileid) (fileid, []dirent) {
	glog.V(3).Infof("readdir: %s [%v]", dir, id)
	c.mu.Lock()
	if id == unknownFileid {
		id = c.ids[dir]
	}
	ents, ok := c.dirents[id]
	c.mu.Unlock()
	if ok {
		return id, ents
	}
	glog.V(3).Infof("opendir: %s", dir)
	d, err := os.Open(dir)
	if err != nil {
		c.mu.Lock()
		c.ids[dir] = invalidFileid
		c.mu.Unlock()
		return invalidFileid, nil
	}
	defer d.Close()
	fi, err := d.Stat()
	if err != nil {
		c.mu.Lock()
		c.ids[dir] = invalidFileid
		c.mu.Unlock()
		return invalidFileid, nil
	}
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
		id = fileid{dev: stat.Dev, ino: stat.Ino}
	}
	names, _ := d.Readdirnames(-1)
	// need sort?
	ents = nil
	var path string
	for _, name := range names {
		path = filepath.Join(dir, name)
		fi, err := os.Lstat(path)
		if err != nil {
			glog.Warningf("readdir %s: %v", name, err)
			ents = append(ents, dirent{name: name})
			continue
		}
		lmode := fi.Mode()
		mode := lmode
		var id fileid
		if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
			id = fileid{dev: stat.Dev, ino: stat.Ino}
		}
		if lmode&os.ModeSymlink == os.ModeSymlink {
			fi, err = os.Stat(path)
			if err != nil {
				glog.Warningf("readdir %s: %v", name, err)
			} else {
				mode = fi.Mode()
				if stat, ok := fi.Sys().(*syscall.Stat_t); ok {
					id = fileid{dev: stat.Dev, ino: stat.Ino}
				}
			}
		}
		ents = append(ents, dirent{id: id, name: name, lmode: lmode, mode: mode})
	}
	glog.V(3).Infof("readdir:%s => %v: %v", dir, id, ents)
	c.mu.Lock()
	c.ids[dir] = id
	c.dirents[id] = ents
	c.mu.Unlock()
	return id, ents
}

// glob searches for files matching pattern in the directory dir
// and appends them to matches. ignore I/O errors.
func (c *fsCacheT) glob(dir, pattern string, matches []string) ([]string, error) {
	_, ents := c.readdir(filepathClean(dir), unknownFileid)
	switch dir {
	case "", string(filepath.Separator):
		// nothing
	default:
		dir += string(filepath.Separator) // add trailing separator back
	}
	for _, ent := range ents {
		matched, err := filepath.Match(pattern, ent.name)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, dir+ent.name)
		}
	}
	return matches, nil
}

func (c *fsCacheT) Glob(pat string) ([]string, error) {
	// TODO(ukai): expand ~ to user's home directory.
	// TODO(ukai): use find cache for glob if exists
	// or use wildcardCache for find cache.
	pat = wildcardUnescape(pat)
	dir, file := filepath.Split(pat)
	switch dir {
	case "", string(filepath.Separator):
		// nothing
	default:
		dir = dir[:len(dir)-1] // chop off trailing separator
	}
	if !hasWildcardMeta(dir) {
		return c.glob(dir, file, nil)
	}

	m, err := c.Glob(dir)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, d := range m {
		matches, err = c.glob(d, file, matches)
		if err != nil {
			return nil, err
		}
	}
	return matches, nil
}

func wildcard(w evalWriter, pat string) error {
	files, err := fsCache.Glob(pat)
	if err != nil {
		return err
	}
	for _, file := range files {
		w.writeWordString(file)
	}
	return nil
}

type findOp interface {
	apply(evalWriter, string, dirent) (test bool, prune bool)
}

type findOpName string

func (op findOpName) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	matched, err := filepath.Match(string(op), ent.name)
	if err != nil {
		glog.Warningf("find -name %q: %v", string(op), err)
		return false, false
	}
	return matched, false
}

type findOpType struct {
	mode           os.FileMode
	followSymlinks bool
}

func (op findOpType) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	mode := ent.lmode
	if op.followSymlinks && ent.mode != 0 {
		mode = ent.mode
	}
	return op.mode&mode == op.mode, false
}

type findOpRegular struct {
	followSymlinks bool
}

func (op findOpRegular) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	mode := ent.lmode
	if op.followSymlinks && ent.mode != 0 {
		mode = ent.mode
	}
	return mode.IsRegular(), false
}

type findOpNot struct {
	op findOp
}

func (op findOpNot) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	test, prune := op.op.apply(w, path, ent)
	return !test, prune
}

type findOpAnd []findOp

func (op findOpAnd) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	var prune bool
	for _, o := range op {
		test, p := o.apply(w, path, ent)
		if p {
			prune = true
		}
		if !test {
			return test, prune
		}
	}
	return true, prune
}

type findOpOr struct {
	op1, op2 findOp
}

func (op findOpOr) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	test, prune := op.op1.apply(w, path, ent)
	if test {
		return test, prune
	}
	return op.op2.apply(w, path, ent)
}

type findOpPrune struct{}

func (op findOpPrune) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	return true, true
}

type findOpPrint struct{}

func (op findOpPrint) apply(w evalWriter, path string, ent dirent) (bool, bool) {
	var name string
	if path == "" {
		name = ent.name
	} else if ent.name == "." {
		name = path
	} else {
		name = filepathJoin(path, ent.name)
	}
	glog.V(3).Infof("find print: %s", name)
	w.writeWordString(name)
	return true, false
}

func (c *fsCacheT) find(w evalWriter, fc findCommand, path string, id fileid, depth int, seen map[fileid]string) {
	glog.V(2).Infof("find: path:%s id:%v depth:%d", path, id, depth)
	id, ents := c.readdir(filepathClean(filepathJoin(fc.chdir, path)), id)
	if ents == nil {
		glog.V(1).Infof("find: %s %s not found", fc.chdir, path)
		return
	}
	for _, ent := range ents {
		glog.V(3).Infof("find: path:%s ent:%s depth:%d", path, ent.name, depth)
		_, prune := fc.apply(w, path, ent)
		mode := ent.lmode
		if fc.followSymlinks {
			if mode&os.ModeSymlink == os.ModeSymlink {
				lpath := filepathJoin(path, ent.name)
				if p, ok := seen[ent.id]; ok {
					// stderr?
					glog.Errorf("find: File system loop detected; `%s' is part of the same file system loop as `%s'.", lpath, p)
					return
				}
				seen[ent.id] = lpath
			}
			mode = ent.mode
		}
		if !mode.IsDir() {
			glog.V(3).Infof("find: not dir: %s/%s", path, ent.name)
			continue
		}
		if prune {
			glog.V(3).Infof("find: prune: %s", path)
			continue
		}
		if depth >= fc.depth {
			glog.V(3).Infof("find: depth: %d >= %d", depth, fc.depth)
			continue
		}
		c.find(w, fc, filepathJoin(path, ent.name), ent.id, depth+1, seen)
	}
}

type findCommand struct {
	testdir        string // before chdir
	chdir          string
	finddirs       []string // after chdir
	followSymlinks bool
	ops            []findOp
	depth          int
}

func parseFindCommand(cmd string) (findCommand, error) {
	if !strings.Contains(cmd, "find") {
		return findCommand{}, errNotFind
	}
	fcp := findCommandParser{
		shellParser: shellParser{
			cmd: cmd,
		},
	}
	err := fcp.parse()
	if err != nil {
		return fcp.fc, err
	}
	if len(fcp.fc.finddirs) == 0 {
		fcp.fc.finddirs = append(fcp.fc.finddirs, ".")
	}
	if fcp.fc.chdir != "" {
		fcp.fc.chdir = filepathClean(fcp.fc.chdir)
	}
	if filepath.IsAbs(fcp.fc.chdir) {
		return fcp.fc, errFindAbspath
	}
	for _, dir := range fcp.fc.finddirs {
		if filepath.IsAbs(dir) {
			return fcp.fc, errFindAbspath
		}
	}
	glog.V(3).Infof("find command: %#v", fcp.fc)

	// TODO(ukai): handle this in run() instead of fallback shell.
	_, ents := fsCache.readdir(filepathClean(fcp.fc.testdir), unknownFileid)
	if ents == nil {
		glog.V(1).Infof("find: testdir %s - not dir", fcp.fc.testdir)
		return fcp.fc, errFindNoSuchDir
	}
	_, ents = fsCache.readdir(filepathClean(fcp.fc.chdir), unknownFileid)
	if ents == nil {
		glog.V(1).Infof("find: cd %s: No such file or directory", fcp.fc.chdir)
		return fcp.fc, errFindNoSuchDir
	}

	return fcp.fc, nil
}

func (fc findCommand) run(w evalWriter) {
	glog.V(3).Infof("find: %#v", fc)
	for _, dir := range fc.finddirs {
		seen := make(map[fileid]string)
		id, _ := fsCache.readdir(filepathClean(filepathJoin(fc.chdir, dir)), unknownFileid)
		_, prune := fc.apply(w, dir, dirent{id: id, name: ".", mode: os.ModeDir, lmode: os.ModeDir})
		if prune {
			glog.V(3).Infof("find: prune: %s", dir)
			continue
		}
		if 0 >= fc.depth {
			glog.V(3).Infof("find: depth: 0 >= %d", fc.depth)
			continue
		}
		fsCache.find(w, fc, dir, id, 1, seen)
	}
}

func (fc findCommand) apply(w evalWriter, path string, ent dirent) (test, prune bool) {
	var p bool
	for _, op := range fc.ops {
		test, p = op.apply(w, path, ent)
		if p {
			prune = true
		}
		if !test {
			break
		}
	}
	glog.V(2).Infof("apply path:%s ent:%v => test=%t, prune=%t", path, ent, test, prune)
	return test, prune
}

var (
	errNotFind             = errors.New("not find command")
	errFindBackground      = errors.New("find command: background")
	errFindUnbalancedQuote = errors.New("find command: unbalanced quote")
	errFindDupChdir        = errors.New("find command: dup chdir")
	errFindDupTestdir      = errors.New("find command: dup testdir")
	errFindExtra           = errors.New("find command: extra")
	errFindUnexpectedEnd   = errors.New("find command: unexpected end")
	errFindAbspath         = errors.New("find command: abs path")
	errFindNoSuchDir       = errors.New("find command: no such dir")
)

type findCommandParser struct {
	fc findCommand
	shellParser
}

func (p *findCommandParser) parse() error {
	p.fc.depth = 1<<31 - 1 // max int32
	var hasIf bool
	var hasFind bool
	for {
		tok, err := p.token()
		if err == io.EOF || tok == "" {
			if !hasFind {
				return errNotFind
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch tok {
		case "cd":
			if p.fc.chdir != "" {
				return errFindDupChdir
			}
			p.fc.chdir, err = p.token()
			if err != nil {
				return err
			}
			err = p.expect(";", "&&")
			if err != nil {
				return err
			}
		case "if":
			err = p.expect("[")
			if err != nil {
				return err
			}
			if hasIf {
				return errFindDupTestdir
			}
			err = p.parseTest()
			if err != nil {
				return err
			}
			err = p.expectSeq("]", ";", "then")
			if err != nil {
				return err
			}
			hasIf = true
		case "test":
			if hasIf {
				return errFindDupTestdir
			}
			err = p.parseTest()
			if err != nil {
				return err
			}
			err = p.expect("&&")
			if err != nil {
				return err
			}
		case "find":
			err = p.parseFind()
			if err != nil {
				return err
			}
			if hasIf {
				err = p.expect("fi")
				if err != nil {
					return err
				}
			}
			tok, err = p.token()
			if err != io.EOF || tok != "" {
				return errFindExtra
			}
			hasFind = true
			return nil
		}
	}
}

func (p *findCommandParser) parseTest() error {
	if p.fc.testdir != "" {
		return errFindDupTestdir
	}
	err := p.expect("-d")
	if err != nil {
		return err
	}
	p.fc.testdir, err = p.token()
	return err
}

func (p *findCommandParser) parseFind() error {
	for {
		tok, err := p.token()
		if err == io.EOF || tok == "" || tok == ";" {
			var print findOpPrint
			if len(p.fc.ops) == 0 || p.fc.ops[len(p.fc.ops)-1] != print {
				p.fc.ops = append(p.fc.ops, print)
			}
			return nil
		}
		if err != nil {
			return err
		}
		if tok != "" && (tok[0] == '-' || tok == "\\(") {
			p.unget(tok)
			op, err := p.parseFindCond()
			if err != nil {
				return err
			}
			if op != nil {
				p.fc.ops = append(p.fc.ops, op)
			}
			continue
		}
		p.fc.finddirs = append(p.fc.finddirs, tok)
	}
}

func (p *findCommandParser) parseFindCond() (findOp, error) {
	return p.parseExpr()
}

func (p *findCommandParser) parseExpr() (findOp, error) {
	op, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, nil
	}
	for {
		tok, err := p.token()
		if err == io.EOF || tok == "" {
			return op, nil
		}
		if err != nil {
			return nil, err
		}
		if tok != "-or" && tok != "-o" {
			p.unget(tok)
			return op, nil
		}
		op2, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		op = findOpOr{op, op2}
	}
}

func (p *findCommandParser) parseTerm() (findOp, error) {
	op, err := p.parseFact()
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, nil
	}
	var ops []findOp
	ops = append(ops, op)
	for {
		tok, err := p.token()
		if err == io.EOF || tok == "" {
			if len(ops) == 1 {
				return ops[0], nil
			}
			return findOpAnd(ops), nil
		}
		if err != nil {
			return nil, err
		}
		if tok != "-and" && tok != "-a" {
			p.unget(tok)
		}
		op, err = p.parseFact()
		if err != nil {
			return nil, err
		}
		if op == nil {
			if len(ops) == 1 {
				return ops[0], nil
			}
			return findOpAnd(ops), nil
		}
		ops = append(ops, op) // findAndOp?
	}
}

func (p *findCommandParser) parseFact() (findOp, error) {
	tok, err := p.token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case "-L":
		p.fc.followSymlinks = true
		return nil, nil
	case "-prune":
		return findOpPrune{}, nil
	case "-print":
		return findOpPrint{}, nil
	case "-maxdepth":
		tok, err = p.token()
		if err != nil {
			return nil, err
		}
		i, err := strconv.ParseInt(tok, 10, 32)
		if err != nil {
			return nil, err
		}
		if i < 0 {
			return nil, fmt.Errorf("find commnad: -maxdepth negative: %d", i)
		}
		p.fc.depth = int(i)
		return nil, nil
	case "-not", "\\!":
		op, err := p.parseFact()
		if err != nil {
			return nil, err
		}
		return findOpNot{op}, nil
	case "\\(":
		op, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		err = p.expect("\\)")
		if err != nil {
			return nil, err
		}
		return op, nil
	case "-name":
		tok, err = p.token()
		if err != nil {
			return nil, err
		}
		return findOpName(tok), nil
	case "-type":
		tok, err = p.token()
		if err != nil {
			return nil, err
		}
		var m os.FileMode
		switch tok {
		case "b":
			m = os.ModeDevice
		case "c":
			m = os.ModeDevice | os.ModeCharDevice
		case "d":
			m = os.ModeDir
		case "p":
			m = os.ModeNamedPipe
		case "l":
			m = os.ModeSymlink
		case "f":
			return findOpRegular{p.fc.followSymlinks}, nil
		case "s":
			m = os.ModeSocket
		default:
			return nil, fmt.Errorf("find command: unsupported -type %s", tok)
		}
		return findOpType{m, p.fc.followSymlinks}, nil
	case "-o", "-or", "-a", "-and":
		p.unget(tok)
		return nil, nil
	default:
		if tok != "" && tok[0] == '-' {
			return nil, fmt.Errorf("find command: unsupported %s", tok)
		}
		p.unget(tok)
		return nil, nil
	}
}

type findleavesCommand struct {
	name     string
	dirs     []string
	prunes   []string
	mindepth int
}

func parseFindleavesCommand(cmd string) (findleavesCommand, error) {
	if !strings.Contains(cmd, "build/tools/findleaves.py") {
		return findleavesCommand{}, errNotFindleaves
	}
	fcp := findleavesCommandParser{
		shellParser: shellParser{
			cmd: cmd,
		},
	}
	err := fcp.parse()
	if err != nil {
		return fcp.fc, err
	}
	glog.V(3).Infof("findleaves command: %#v", fcp.fc)
	return fcp.fc, nil
}

func (fc findleavesCommand) run(w evalWriter) {
	glog.V(3).Infof("findleaves: %#v", fc)
	for _, dir := range fc.dirs {
		seen := make(map[fileid]string)
		id, _ := fsCache.readdir(filepathClean(dir), unknownFileid)
		fc.walk(w, dir, id, 1, seen)
	}
}

func (fc findleavesCommand) walk(w evalWriter, dir string, id fileid, depth int, seen map[fileid]string) {
	glog.V(3).Infof("findleaves walk: dir:%d id:%v depth:%d", dir, id, depth)
	id, ents := fsCache.readdir(filepathClean(dir), id)
	var subdirs []dirent
	for _, ent := range ents {
		if ent.mode.IsDir() {
			if fc.isPrune(ent.name) {
				glog.V(3).Infof("findleaves prune %s in %s", ent.name, dir)
				continue
			}
			subdirs = append(subdirs, ent)
			continue
		}
		if depth < fc.mindepth {
			glog.V(3).Infof("findleaves depth=%d mindepth=%d", depth, fc.mindepth)
			continue
		}
		if ent.name == fc.name {
			glog.V(2).Infof("findleaves %s in %s", ent.name, dir)
			w.writeWordString(filepathJoin(dir, ent.name))
			// no recurse subdirs
			return
		}
	}
	for _, subdir := range subdirs {
		if subdir.lmode&os.ModeSymlink == os.ModeSymlink {
			lpath := filepathJoin(dir, subdir.name)
			if p, ok := seen[subdir.id]; ok {
				// symlink loop detected.
				glog.Errorf("findleaves: loop detected %q was %q", lpath, p)
				continue
			}
			seen[subdir.id] = lpath
		}
		fc.walk(w, filepathJoin(dir, subdir.name), subdir.id, depth+1, seen)
	}
}

func (fc findleavesCommand) isPrune(name string) bool {
	for _, p := range fc.prunes {
		if p == name {
			return true
		}
	}
	return false
}

var (
	errNotFindleaves        = errors.New("not findleaves command")
	errFindleavesEmptyPrune = errors.New("findleaves: empty prune")
	errFindleavesNoFilename = errors.New("findleaves: no filename")
)

type findleavesCommandParser struct {
	fc findleavesCommand
	shellParser
}

func (p *findleavesCommandParser) parse() error {
	var args []string
	p.fc.mindepth = -1
	tok, err := p.token()
	if err != nil {
		return err
	}
	if tok != "build/tools/findleaves.py" {
		return errNotFindleaves
	}
	for {
		tok, err := p.token()
		if err == io.EOF || tok == "" {
			break
		}
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(tok, "--prune="):
			prune := filepath.Base(strings.TrimPrefix(tok, "--prune="))
			if prune == "" {
				return errFindleavesEmptyPrune
			}
			p.fc.prunes = append(p.fc.prunes, prune)
		case strings.HasPrefix(tok, "--mindepth="):
			md := strings.TrimPrefix(tok, "--mindepth=")
			i, err := strconv.ParseInt(md, 10, 32)
			if err != nil {
				return err
			}
			p.fc.mindepth = int(i)
		default:
			args = append(args, tok)
		}
	}
	if len(args) < 2 {
		return errFindleavesNoFilename
	}
	p.fc.dirs, p.fc.name = args[:len(args)-1], args[len(args)-1]
	return nil
}
