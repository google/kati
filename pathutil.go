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
	Logf("android find cache in dir: %s i=%d/%d", dir, i, len(c.files))
	for ; i < len(c.files); i++ {
		if c.files[i].path != dir && !strings.HasPrefix(c.files[i].path, dir+"/") {
			Logf("android find cache in dir: %s different prefix: %s", dir, c.files[i].path)
			break
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
		Logf("android find cache in dir: %s=> %s", dir, name)
	}
}
