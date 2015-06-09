package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var wildcardCache = make(map[string][]string)

func wildcard(sw *ssvWriter, pat string) {
	if useWildcardCache {
		// TODO(ukai): make sure it didn't chdir?
		if files, ok := wildcardCache[pat]; ok {
			for _, file := range files {
				sw.WriteString(file)
			}
			return
		}
	}
	if strings.Contains(pat, "..") {
		// For some reason, go's Glob normalizes
		// foo/../bar to bar. We ask shell to expand
		// a glob to avoid this.
		cmdline := []string{"/bin/sh", "-c", "/bin/ls -d " + pat}
		cmd := exec.Cmd{
			Path: cmdline[0],
			Args: cmdline,
		}
		// Ignore errors.
		out, _ := cmd.Output()
		if len(trimSpaceBytes(out)) > 0 {
			out = formatCommandOutput(out)
			sw.Write(out)
		}
		if useWildcardCache {
			ws := newWordScanner(out)
			var files []string
			for ws.Scan() {
				files = append(files, string(ws.Bytes()))
			}
			wildcardCache[pat] = files
		}
		return
	}
	files, err := filepath.Glob(pat)
	if err != nil {
		panic(err)
	}
	for _, file := range files {
		sw.WriteString(file)
	}
	if useWildcardCache {
		wildcardCache[pat] = files
	}
}

type fileInfo struct {
	path string
	mode os.FileMode
}

type androidFindCacheT struct {
	once     sync.Once
	mu       sync.Mutex
	ok       bool
	files    []fileInfo
	scanTime time.Duration
}

var (
	androidFindCache androidFindCacheT
)

func (c *androidFindCacheT) ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ok
}

func (c *androidFindCacheT) init() {
	c.once.Do(func() {
		c.mu.Lock()
		go c.start()
	})
}

func (c *androidFindCacheT) start() {
	defer c.mu.Unlock()
	t := time.Now()
	defer func() {
		c.scanTime = time.Since(t)
		Logf("android find cache scan: %v", c.scanTime)
	}()

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		c.files = append(c.files, fileInfo{
			path: strings.TrimPrefix(path, "./"),
			mode: info.Mode(),
		})
		return nil
	})
	if err != nil {
		Logf("error in adnroid find cache: %v", err)
		c.ok = false
		return
	}
	sort.Sort(fileInfoByName(c.files))
	for i, fi := range c.files {
		Logf("android find cache: %d: %s %v", i, fi.path, fi.mode)
	}
	c.ok = true
}

type fileInfoByName []fileInfo

func (f fileInfoByName) Len() int      { return len(f) }
func (f fileInfoByName) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f fileInfoByName) Less(i, j int) bool {
	return f[i].path < f[j].path
}

// if [ -d $1 ] ; then cd $1 ; find ./ -not -name '.*' -and -type f -and -not -type l ; fi
func (c *androidFindCacheT) findInDir(sw *ssvWriter, dir string) {
	dir = strings.TrimPrefix(dir, "./")
	i := sort.Search(len(c.files), func(i int) bool {
		return c.files[i].path >= dir
	})
	Logf("android find in dir cache: %s i=%d/%d", dir, i, len(c.files))
	for ; i < len(c.files); i++ {
		if c.files[i].path != dir {
			if !strings.HasPrefix(c.files[i].path, dir) {
				Logf("android find in dir cache: %s different prefix at %d: %s", dir, i, c.files[i].path)
				break
			}
			if !strings.HasPrefix(c.files[i].path, dir+"/") {
				continue
			}
		}
		// -not -name '.*'
		if strings.HasPrefix(filepath.Base(c.files[i].path), ".") {
			continue
		}
		// -type f and -not -type l
		// regular type and not symlink
		if !c.files[i].mode.IsRegular() {
			continue
		}
		name := strings.TrimPrefix(c.files[i].path, dir+"/")
		name = "./" + name
		sw.WriteString(name)
		Logf("android find in dir cache: %s=> %s", dir, name)
	}
}

// cd ${LOCAL_PATH} ; find -L $1 -name "*.java" -and -not -name ".*"
// returns false if symlink is found.
func (c *androidFindCacheT) findJavaInDir(sw *ssvWriter, chdir string, root string) bool {
	chdir = strings.TrimPrefix(chdir, "./")
	dir := filepath.Join(chdir, root)
	i := sort.Search(len(c.files), func(i int) bool {
		return c.files[i].path >= dir
	})
	Logf("android find java in dir cache: %s i=%d/%d", dir, i, len(c.files))
	start := i
	end := len(c.files)
	// check symlinks
	for ; i < len(c.files); i++ {
		if c.files[i].path != dir {
			if !strings.HasPrefix(c.files[i].path, dir) {
				Logf("android find in dir cache: %s different prefix at %d: %s", dir, i, c.files[i].path)
				end = i
				break
			}
			if !strings.HasPrefix(c.files[i].path, dir+"/") {
				continue
			}
		}
		if c.files[i].mode&os.ModeSymlink == os.ModeSymlink {
			Logf("android find java in dir cache: detect symlink %s %v", c.files[i].path, c.files[i].mode)
			return false
		}
	}

	// no symlinks
	for i := start; i < end; i++ {
		if c.files[i].path != dir && !strings.HasPrefix(c.files[i].path, dir+"/") {
			continue
		}
		base := filepath.Base(c.files[i].path)
		// -name "*.java"
		if filepath.Ext(base) != ".java" {
			continue
		}
		// -not -name ".*"
		if strings.HasPrefix(base, ".") {
			continue
		}
		name := strings.TrimPrefix(c.files[i].path, chdir+"/")
		sw.WriteString(name)
		Logf("android find java in dir cache: %s=> %s", dir, name)
	}
	return true
}
