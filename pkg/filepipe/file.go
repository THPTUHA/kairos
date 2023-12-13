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

// File is the virtual-file API used by Gonzo.
// This method `State` is provided for interoperability with os.File,
// so consider using the following interface for a Gonzo and os.File
// compatiable API
//
//	type File interface {
//	  io.ReadCloser
//	  Stat() (os.FileInfo, error)
//	}
type File interface {
	io.ReadCloser
	Stat() (os.FileInfo, error)
	FileInfo() FileInfo
}

// NewFile returns a file using the provided io.ReadCloser and FileInfo.
func NewFile(rc io.ReadCloser, fi FileInfo) File {
	if fi == nil {
		fi = &fileinfo{}
	}
	return file{rc, fi}
}

// FileInfo is a mutuable superset of os.FileInfo
// with the additon of "Base"
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

// Stage is a function that takes a context, a channel of files to read and
// an output chanel.
// There is no correlation between a stages input and output, a stage may
// decided to pass the same files after transofrmation or generate new files
// based on the input or drop files.
//
// A stage must not close the output channel based on the simple
// "Don't close it if you don't own it" principle.
// A stage must either pass on a file or call the `Close` method on it.
type Stage func(context.Context, <-chan File, chan<- File) error

// Pipe handles stages and talks to other pipes.
type Pipe interface {
	Context() context.Context
	Files() <-chan File
	Pipe(stages ...Stage) Pipe
	Then(stages ...Stage) error
	Wait() error
}

// NewPipe returns a pipe using the context provided and
// channel of files. If you don't need a context, use context.Background()
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

// FileInfoFrom createa a FileInfo from an os.FileInfo.
// Useful when working with os.File or other APIs that
// mimics http.FileSystem.
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

// NewFileInfo create a new empty FileInfo.
func NewFileInfo() FileInfo {
	return &fileinfo{
		lock: &sync.RWMutex{},
	}
}

// Fileinfo implements os.fileinfo.
type fileinfo struct {
	lock    *sync.RWMutex
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
	sys     interface{}

	base string //base, usually glob.Base
	//XXX: For consideration.
	//Cwd  string //Where are we?
	//Path string //Full path.

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

//Extension

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
