package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/THPTUHA/kairos/pkg/filepipe"
	"github.com/THPTUHA/kairos/pkg/glob"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

const (
	maxBufSize = 256000
)

var ErrIsDir = errors.New("path is a directory")

type File struct {
}

func (s *File) Execute(args *proto.ExecuteRequest, cb plugin.StatusHelper) (*proto.ExecuteResponse, error) {
	out, err := s.ExecuteImpl(args, cb)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *File) ExecuteImpl(args *proto.ExecuteRequest, cb plugin.StatusHelper) ([]byte, error) {
	path := args.Config["path"]
	method := args.Config["method"]
	input := args.Config["input"]

	if input != "" {
		var mp map[string]interface{}
		err := json.Unmarshal([]byte(input), &mp)
		if err != nil {
			return nil, err
		}
		if mp["path"] != "" {
			path = mp["path"].(string)
		}

		if mp["method"] != "" {
			method = mp["method"].(string)
		}
	}

	if path == "" {
		return nil, errors.New("Path file empty")
	}

	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if method == "read" {
		path = filepath.Join(dir, path)
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		bufferSize := 1000
		buffer := make([]byte, bufferSize)

		for {
			_, err := file.Read(buffer)
			if err == io.EOF {
				break
			}
			_, err = cb.Update(buffer, true)
			if err != nil {
				fmt.Println(err)
				break
			}
		}

		return nil, nil
	}
	return nil, nil

}

func Read(path string) (filepipe.File, error) {
	Stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if Stat.IsDir() {
		return nil, ErrIsDir
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return filepipe.NewFile(f, filepipe.FileInfoFrom(Stat)), nil
}

func Src(ctx context.Context, globs ...string) filepipe.Pipe {

	ctx, cancel := context.WithCancel(ctx)

	files := make(chan filepipe.File)
	pipe := filepipe.NewPipe(ctx, files)

	go func() {

		var err error
		defer close(files)

		fileslist, err := glob.Glob(globs...)
		if err != nil {
			return
		}

		for mp := range fileslist {

			var (
				file filepipe.File
				base = glob.Dir(mp.Glob)
				name = mp.Name
			)

			file, err = Read(mp.Name)
			ctx = context.WithValue(ctx, "file", name)

			if err == ErrIsDir {
				continue

			}

			if err != nil {
				cancel()
				return
			}

			file.FileInfo().SetBase(base)
			file.FileInfo().SetName(name)
			files <- file

		}

	}()

	return pipe
}

// Dest writes the files from the input channel to the dst folder and closes the files.
// It never returns Files.
func Dest(dst string) filepipe.Stage {
	return func(ctx context.Context, files <-chan filepipe.File, out chan<- filepipe.File) error {

		for {
			select {
			case file, ok := <-files:
				if !ok {
					return nil
				}

				name := file.FileInfo().Name()
				path := filepath.Join(dst, filepath.Dir(name))
				err := os.MkdirAll(path, 0700)
				if err != nil {
					return err
				}

				if file.FileInfo().IsDir() {
					out <- file
					continue
				}

				content, err := ioutil.ReadAll(file)
				if err != nil {
					file.Close()
					return err
				}

				ctx = context.WithValue(ctx, "path", path)
				err = writeFile(filepath.Join(dst, name), content)
				if err != nil {
					return err
				}

				out <- filepipe.NewFile(io.NopCloser(bytes.NewReader(content)), file.FileInfo())

			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func writeFile(path string, content []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(content)
	return err
}
