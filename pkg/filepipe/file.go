package filepipe

import (
	"context"
	"flag"
	"io"
	"os"
	"sync"
	"time"
)

var help = flag.Bool("help", false, "show help")

type File interface {
	io.ReadCloser
	Stat() (os.FileInfo, error)
	FileInfo() FileInfo
}

func NewFile(rc io.ReadCloser, fi FileInfo) File {
	if fi == nil {
		fi = &fileinfo{}
	}
	return file{rc, fi}
}

type FileInfo interface {
	Name() string
	Size() int64
	Mode() os.FileMode
	ModTime() time.Time
	IsDir() bool
	Sys() interface{}

	Base() string

	SetName(string)
	SetSize(int64)
	SetMode(os.FileMode)
	SetModTime(time.Time)
	SetIsDir(bool)

	SetBase(string)
}

type Stage func(context.Context, <-chan File, chan<- File) error

type Pipe interface {
	Context() context.Context
	Files() <-chan File
	Pipe(stages ...Stage) Pipe
	Then(stages ...Stage) error
	Wait() error
}

func NewPipe(ctx context.Context, files <-chan File) Pipe {
	return pipe{
		context: ctx,
		files:   files,
	}
}

type file struct {
	io.ReadCloser
	fileinfo FileInfo
}

func (f file) Stat() (os.FileInfo, error) {
	return f.fileinfo, nil
}

func (f file) FileInfo() FileInfo {
	return f.fileinfo
}

func FileInfoFrom(fi os.FileInfo) FileInfo {
	return &fileinfo{
		&sync.RWMutex{},
		fi.Name(),
		fi.Size(),
		fi.Mode(),
		fi.ModTime(),
		fi.IsDir(),
		fi.Sys(),
		"",
	}

}

func NewFileInfo() FileInfo {
	return &fileinfo{
		lock: &sync.RWMutex{},
	}
}

type fileinfo struct {
	lock    *sync.RWMutex
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}

	base string //base, usually glob.Base
}

func (f fileinfo) Name() string {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.name
}
func (f fileinfo) Size() int64 {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.size
}

func (f fileinfo) Mode() os.FileMode {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.mode
}

func (f fileinfo) ModTime() time.Time {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.modTime
}

func (f fileinfo) IsDir() bool {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.isDir
}

func (f fileinfo) Sys() interface{} {
	f.lock.RLock()
	defer f.lock.RUnlock()
	return f.sys
}

func (f *fileinfo) SetName(name string) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.name = name
}

func (f *fileinfo) SetSize(size int64) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.size = size
}

func (f *fileinfo) SetMode(mod os.FileMode) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.mode = mod
}

func (f *fileinfo) SetModTime(time time.Time) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.modTime = time
}
func (f *fileinfo) SetIsDir(isdir bool) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.isDir = isdir
}
func (f *fileinfo) SetSys(sys interface{}) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.sys = sys
}

func (f fileinfo) Base() string {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.base
}

func (f *fileinfo) SetBase(base string) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.base = base
}
