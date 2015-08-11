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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type mockfs struct {
	id       fileid
	ofscache *fsCacheT
}

func newFS() *mockfs {
	fs := &mockfs{
		ofscache: fsCache,
	}
	fsCache = &fsCacheT{
		ids:     make(map[string]fileid),
		dirents: make(map[fileid][]dirent),
	}
	fsCache.ids["."] = fs.dir(".").id
	return fs
}

func (m *mockfs) dump(t *testing.T) {
	t.Log("fs ids:")
	for name, id := range fsCache.ids {
		t.Logf(" %q=%v", name, id)
	}
	t.Log("fs dirents:")
	for id, ents := range fsCache.dirents {
		t.Logf(" %v:", id)
		for _, ent := range ents {
			t.Logf("  %#v", ent)
		}
	}
}

func (m *mockfs) close() {
	fsCache = m.ofscache
}

func (m *mockfs) dirent(name string, mode os.FileMode) dirent {
	id := m.id
	m.id.ino++
	return dirent{id: id, name: name, mode: mode, lmode: mode}
}

func (m *mockfs) addent(name string, ent dirent) {
	dir, name := filepath.Split(name)
	dir = strings.TrimSuffix(dir, string(filepath.Separator))
	if dir == "" {
		dir = "."
	}
	di, ok := fsCache.ids[dir]
	if !ok {
		if dir == "." {
			panic(". not found:" + name)
		}
		de := m.add(m.dir, dir)
		fsCache.ids[dir] = de.id
		di = de.id
	}
	for _, e := range fsCache.dirents[di] {
		if e.name == ent.name {
			return
		}
	}
	fsCache.dirents[di] = append(fsCache.dirents[di], ent)
}

func (m *mockfs) add(t func(string) dirent, name string) dirent {
	ent := t(filepath.Base(name))
	m.addent(name, ent)
	return ent
}

func (m *mockfs) symlink(name string, ent dirent) {
	lent := ent
	lent.lmode = os.ModeSymlink
	lent.name = filepath.Base(name)
	m.addent(name, lent)
}

func (m *mockfs) dirref(name string) dirent {
	id := fsCache.ids[name]
	return dirent{id: id, name: filepath.Base(name), mode: os.ModeDir, lmode: os.ModeDir}
}

func (m *mockfs) notfound() dirent        { return dirent{id: invalidFileid} }
func (m *mockfs) dir(name string) dirent  { return m.dirent(name, os.ModeDir) }
func (m *mockfs) file(name string) dirent { return m.dirent(name, os.FileMode(0644)) }

func TestFilepathClean(t *testing.T) {
	fs := newFS()
	defer fs.close()
	di := fs.add(fs.dir, "dir")
	fs.symlink("link", di)

	fs.dump(t)

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "foo", want: "foo"},
		{path: ".", want: "."},
		{path: "./", want: "."},
		{path: ".///", want: "."},
		{path: "", want: "."},
		{path: "foo/bar", want: "foo/bar"},
		{path: "./foo", want: "foo"},
		{path: "foo///", want: "foo"},
		{path: "foo//bar", want: "foo/bar"},
		{path: "foo/../bar", want: "foo/../bar"},   // foo doesn't exist
		{path: "dir/../bar", want: "bar"},          // dir is real dir
		{path: "link/../bar", want: "link/../bar"}, // link is symlink
		{path: "foo/./bar", want: "foo/bar"},
		{path: "/foo/bar", want: "/foo/bar"},
	} {
		if got, want := filepathClean(tc.path), tc.want; got != want {
			t.Errorf("filepathClean(%q)=%q; want=%q", tc.path, got, want)
		}
	}
}

func TestParseFindCommand(t *testing.T) {
	fs := newFS()
	defer fs.close()
	fs.add(fs.dir, "testdir")

	maxdepth := 1<<31 - 1
	for _, tc := range []struct {
		cmd  string
		want findCommand
	}{
		{
			cmd: "find testdir",
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: "find .",
			want: findCommand{
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: "find ",
			want: findCommand{
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: "find testdir/../testdir",
			want: findCommand{
				finddirs: []string{"testdir/../testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: "find testdir -print",
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: "find testdir -name foo",
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpName("foo"), findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "file1"`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpName("file1"), findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1"`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpName("*1"), findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -and -name "file*"`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpAnd{findOpName("*1"), findOpName("file*")}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -or -name "file*"`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpName("file*")}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -or -type f`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpRegular{}}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -or -not -type f`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpNot{findOpRegular{}}}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -or \! -type f`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpNot{findOpRegular{}}}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -or -type d`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpType{mode: os.ModeDir}}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -or -type l`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpType{mode: os.ModeSymlink}}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name "*1" -a -type l -o -name "dir*"`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpAnd([]findOp{findOpName("*1"), findOpType{mode: os.ModeSymlink}}), findOpName("dir*")}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir \( -name "dir*" -o -name "*1" \) -a -type f`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpAnd([]findOp{findOpOr{findOpName("dir*"), findOpName("*1")}, findOpRegular{}}), findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `cd testdir && find`,
			want: findCommand{
				chdir:    "testdir",
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `test -d testdir && find testdir`,
			want: findCommand{
				testdir:  "testdir",
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `if [ -d testdir ] ; then find testdir ; fi`,
			want: findCommand{
				testdir:  "testdir",
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `if [ -d testdir ]; then find testdir; fi`,
			want: findCommand{
				testdir:  "testdir",
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `if [ -d testdir ]; then cd testdir && find .; fi`,
			want: findCommand{
				chdir:    "testdir",
				testdir:  "testdir",
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir -name dir2 -prune -o -name file1`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpAnd([]findOp{findOpName("dir2"), findOpPrune{}}), findOpName("file1")}, findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find testdir testdir`,
			want: findCommand{
				finddirs: []string{"testdir", "testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
		},
		{
			cmd: `find -L testdir -type f`,
			want: findCommand{
				finddirs:       []string{"testdir"},
				followSymlinks: true,
				ops:            []findOp{findOpRegular{followSymlinks: true}, findOpPrint{}},
				depth:          maxdepth,
			},
		},
		{
			cmd: `cd testdir; find -L . -type f`,
			want: findCommand{
				chdir:          "testdir",
				finddirs:       []string{"."},
				followSymlinks: true,
				ops:            []findOp{findOpRegular{followSymlinks: true}, findOpPrint{}},
				depth:          maxdepth,
			},
		},
		{
			cmd: `find testdir -maxdepth 1`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    1,
			},
		},
		{
			cmd: `find testdir -maxdepth 0`,
			want: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    0,
			},
		},
	} {
		fc, err := parseFindCommand(tc.cmd)
		if err != nil {
			t.Errorf("parseFindCommand(%q)=_, %v; want=_, <nil>", tc.cmd, err)
			continue
		}
		if got, want := fc, tc.want; !reflect.DeepEqual(got, want) {
			t.Errorf("parseFindCommand(%q)=%#v\n want=%#v\n", tc.cmd, got, want)
		}
	}

}

func TestParseFindCommandFail(t *testing.T) {
	for _, cmd := range []string{
		`find testdir -maxdepth hoge`,
		`find testdir -maxdepth 1hoge`,
		`find testdir -maxdepth -1`,
	} {
		_, err := parseFindCommand(cmd)
		if err == nil {
			t.Errorf("parseFindCommand(%q)=_, <nil>; want=_, err", cmd)
		}
	}
}

func TestFind(t *testing.T) {
	fs := newFS()
	defer fs.close()
	fs.add(fs.file, "Makefile")
	fs.add(fs.file, "testdir/file1")
	fs.add(fs.file, "testdir/file2")
	file1 := fs.add(fs.file, "testdir/dir1/file1")
	dir1 := fs.dirref("testdir/dir1")
	fs.add(fs.file, "testdir/dir1/file2")
	fs.add(fs.file, "testdir/dir2/file1")
	fs.add(fs.file, "testdir/dir2/file2")
	fs.symlink("testdir/dir2/link1", file1)
	fs.symlink("testdir/dir2/link2", dir1)
	fs.symlink("testdir/dir2/link3", fs.notfound())

	fs.dump(t)

	maxdepth := 1<<31 - 1
	for _, tc := range []struct {
		fc   findCommand
		want string
	}{
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `. ./Makefile ./testdir ./testdir/file1 ./testdir/file2 ./testdir/dir1 ./testdir/dir1/file1 ./testdir/dir1/file2 ./testdir/dir2 ./testdir/dir2/file1 ./testdir/dir2/file2 ./testdir/dir2/link1 ./testdir/dir2/link2 ./testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"./"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `./ ./Makefile ./testdir ./testdir/file1 ./testdir/file2 ./testdir/dir1 ./testdir/dir1/file1 ./testdir/dir1/file2 ./testdir/dir2 ./testdir/dir2/file1 ./testdir/dir2/file2 ./testdir/dir2/link1 ./testdir/dir2/link2 ./testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{".///"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `./// .///Makefile .///testdir .///testdir/file1 .///testdir/file2 .///testdir/dir1 .///testdir/dir1/file1 .///testdir/dir1/file2 .///testdir/dir2 .///testdir/dir2/file1 .///testdir/dir2/file2 .///testdir/dir2/link1 .///testdir/dir2/link2 .///testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"./."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `./. ././Makefile ././testdir ././testdir/file1 ././testdir/file2 ././testdir/dir1 ././testdir/dir1/file1 ././testdir/dir1/file2 ././testdir/dir2 ././testdir/dir2/file1 ././testdir/dir2/file2 ././testdir/dir2/link1 ././testdir/dir2/link2 ././testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"././"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `././ ././Makefile ././testdir ././testdir/file1 ././testdir/file2 ././testdir/dir1 ././testdir/dir1/file1 ././testdir/dir1/file2 ././testdir/dir2 ././testdir/dir2/file1 ././testdir/dir2/file2 ././testdir/dir2/link1 ././testdir/dir2/link2 ././testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir/../testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/../testdir testdir/../testdir/file1 testdir/../testdir/file2 testdir/../testdir/dir1 testdir/../testdir/dir1/file1 testdir/../testdir/dir1/file2 testdir/../testdir/dir2 testdir/../testdir/dir2/file1 testdir/../testdir/dir2/file2 testdir/../testdir/dir2/link1 testdir/../testdir/dir2/link2 testdir/../testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpName("foo"), findOpPrint{}},
				depth:    maxdepth,
			},
			want: ``,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpName("file1"), findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/dir1/file1 testdir/dir2/file1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpAnd{findOpName("*1"), findOpName("file*")}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/dir1/file1 testdir/dir2/file1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpName("file*")}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpRegular{}}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpNot{findOpRegular{}}}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir testdir/file1 testdir/dir1 testdir/dir1/file1 testdir/dir2 testdir/dir2/file1 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpType{mode: os.ModeDir}}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir testdir/file1 testdir/dir1 testdir/dir1/file1 testdir/dir2 testdir/dir2/file1 testdir/dir2/link1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpName("*1"), findOpType{mode: os.ModeSymlink}}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/dir1 testdir/dir1/file1 testdir/dir2/file1 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpAnd([]findOp{findOpName("*1"), findOpType{mode: os.ModeSymlink}}), findOpName("dir*")}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/dir1 testdir/dir2 testdir/dir2/link1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpAnd([]findOp{findOpName("*1"), findOpType{mode: os.ModeSymlink}}), findOpName("dir*")}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/dir1 testdir/dir2 testdir/dir2/link1`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpAnd([]findOp{findOpOr{findOpName("dir*"), findOpName("*1")}, findOpRegular{}}), findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/dir1/file1 testdir/dir2/file1`,
		},
		{
			fc: findCommand{
				chdir:    "testdir",
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `. ./file1 ./file2 ./dir1 ./dir1/file1 ./dir1/file2 ./dir2 ./dir2/file1 ./dir2/file2 ./dir2/link1 ./dir2/link2 ./dir2/link3`,
		},
		{
			fc: findCommand{
				chdir:    "testdir",
				finddirs: []string{"../testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `../testdir ../testdir/file1 ../testdir/file2 ../testdir/dir1 ../testdir/dir1/file1 ../testdir/dir1/file2 ../testdir/dir2 ../testdir/dir2/file1 ../testdir/dir2/file2 ../testdir/dir2/link1 ../testdir/dir2/link2 ../testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				testdir:  "testdir",
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				chdir:    "testdir",
				testdir:  "testdir",
				finddirs: []string{"."},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `. ./file1 ./file2 ./dir1 ./dir1/file1 ./dir1/file2 ./dir2 ./dir2/file1 ./dir2/file2 ./dir2/link1 ./dir2/link2 ./dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpOr{findOpAnd([]findOp{findOpName("dir2"), findOpPrune{}}), findOpName("file1")}, findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir/file1 testdir/dir1/file1 testdir/dir2`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir", "testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    maxdepth,
			},
			want: `testdir testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3 testdir testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3`,
		},
		// symlink
		{
			fc: findCommand{
				finddirs:       []string{"testdir"},
				followSymlinks: true,
				ops:            []findOp{findOpRegular{followSymlinks: true}, findOpPrint{}},
				depth:          maxdepth,
			},
			want: `testdir/file1 testdir/file2 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1 testdir/dir2/link2/file1 testdir/dir2/link2/file2`,
		},
		{
			fc: findCommand{
				finddirs:       []string{"testdir"},
				followSymlinks: true,
				ops:            []findOp{findOpType{mode: os.ModeDir, followSymlinks: true}, findOpPrint{}},
				depth:          maxdepth,
			},
			want: `testdir testdir/dir1 testdir/dir2 testdir/dir2/link2`,
		},
		{
			fc: findCommand{
				finddirs:       []string{"testdir"},
				followSymlinks: true,
				ops:            []findOp{findOpType{mode: os.ModeSymlink, followSymlinks: true}, findOpPrint{}},
				depth:          maxdepth,
			},
			want: `testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				chdir:          "testdir",
				finddirs:       []string{"."},
				followSymlinks: true,
				ops:            []findOp{findOpRegular{followSymlinks: true}, findOpPrint{}},
				depth:          maxdepth,
			},
			want: `./file1 ./file2 ./dir1/file1 ./dir1/file2 ./dir2/file1 ./dir2/file2 ./dir2/link1 ./dir2/link2/file1 ./dir2/link2/file2`,
		},
		// maxdepth
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    1,
			},
			want: `testdir testdir/file1 testdir/file2 testdir/dir1 testdir/dir2`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    2,
			},
			want: `testdir testdir/file1 testdir/file2 testdir/dir1 testdir/dir1/file1 testdir/dir1/file2 testdir/dir2 testdir/dir2/file1 testdir/dir2/file2 testdir/dir2/link1 testdir/dir2/link2 testdir/dir2/link3`,
		},
		{
			fc: findCommand{
				finddirs: []string{"testdir"},
				ops:      []findOp{findOpPrint{}},
				depth:    0,
			},
			want: `testdir`,
		},
	} {
		var wb wordBuffer
		tc.fc.run(&wb)
		if got, want := wb.buf.String(), tc.want; got != want {
			t.Errorf("%#v\n got  %q\n want %q", tc.fc, got, want)
		}
	}
}

func TestParseFindleavesCommand(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want findleavesCommand
	}{
		{
			cmd: `build/tools/findleaves.py --prune=out --prune=.repo --prune=.git . CleanSpec.mk`,
			want: findleavesCommand{
				name:     "CleanSpec.mk",
				dirs:     []string{"."},
				prunes:   []string{"out", ".repo", ".git"},
				mindepth: -1,
			},
		},
		{
			cmd: `build/tools/findleaves.py --prune=out --prune=.repo --prune=.git --mindepth=2  art bionic Android.mk`,
			want: findleavesCommand{
				name:     "Android.mk",
				dirs:     []string{"art", "bionic"},
				prunes:   []string{"out", ".repo", ".git"},
				mindepth: 2,
			},
		},
	} {
		fc, err := parseFindleavesCommand(tc.cmd)
		if err != nil {
			t.Errorf("parseFindleavesCommand(%q)=_, %v; want=_, <nil", tc.cmd, err)
			continue
		}
		if got, want := fc, tc.want; !reflect.DeepEqual(got, want) {
			t.Errorf("parseFindleavesCommand(%q)=%#v\n want=%#v\n", tc.cmd, got, want)
		}
	}
}

func TestFindleaves(t *testing.T) {
	fs := newFS()
	defer fs.close()

	fs.add(fs.file, "art/Android.mk")
	fs.add(fs.file, "art/compiler/Android.mk")
	fs.add(fs.file, "art/CleanSpec.mk")
	fs.add(fs.file, "bionic/Android.mk")
	fs.add(fs.file, "bionic/CleanSpec.mk")
	fs.add(fs.file, "bootable/recovery/Android.mk")
	fs.add(fs.file, "bootable/recovery/CleanSpec.mk")
	fs.add(fs.file, "frameworks/base/Android.mk")
	fs.add(fs.file, "frameworks/base/CleanSpec.mk")
	fs.add(fs.file, "frameworks/base/cmds/am/Android.mk")
	fs.add(fs.file, "frameworks/base/cmds/pm/Android.mk")
	fs.add(fs.file, "frameworks/base/location/Android.mk")
	fs.add(fs.file, "frameworks/base/packages/WAPPushManager/CleanSpec.mk")
	fs.add(fs.file, "out/outputfile")
	fs.add(fs.file, "art/.git/index")
	fs.add(fs.file, ".repo/manifests")

	fs.dump(t)

	for _, tc := range []struct {
		fc   findleavesCommand
		want string
	}{
		{
			fc: findleavesCommand{
				name:     "CleanSpec.mk",
				dirs:     []string{"."},
				prunes:   []string{"out", ".repo", ".git"},
				mindepth: -1,
			},
			want: `./art/CleanSpec.mk ./bionic/CleanSpec.mk ./bootable/recovery/CleanSpec.mk ./frameworks/base/CleanSpec.mk`,
		},
		{
			fc: findleavesCommand{
				name:     "Android.mk",
				dirs:     []string{"art", "bionic", "frameworks/base"},
				prunes:   []string{"out", ".repo", ".git"},
				mindepth: 2,
			},
			want: `art/compiler/Android.mk frameworks/base/cmds/am/Android.mk frameworks/base/cmds/pm/Android.mk frameworks/base/location/Android.mk`,
		},
		{
			fc: findleavesCommand{
				name:     "Android.mk",
				dirs:     []string{"art", "bionic", "frameworks/base"},
				prunes:   []string{"out", ".repo", ".git"},
				mindepth: 3,
			},
			want: `frameworks/base/cmds/am/Android.mk frameworks/base/cmds/pm/Android.mk`,
		},
	} {
		var wb wordBuffer
		tc.fc.run(&wb)
		if got, want := wb.buf.String(), tc.want; got != want {
			t.Errorf("%#v\n got  %q\n want %q", tc.fc, got, want)
		}
	}
}
