package main

import (
	"fmt"
	"io"
	"os"

	"github.com/THPTUHA/kairos/pkg/circbuf"
	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

const (
	maxBufSize = 256000
)

type File struct {
}

func (s *File) Execute(args *proto.ExecuteRequest, cb kplugin.StatusHelper) (*proto.ExecuteResponse, error) {
	out, err := s.ExecuteImpl(args)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func (s *File) ExecuteImpl(args *proto.ExecuteRequest) ([]byte, error) {
	output, _ := circbuf.NewBuffer(maxBufSize)
	output.Write([]byte("OK: ABABY"))
	path := args.Config["path"]
	if path != "" {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		bufferSize := 20
		buffer := make([]byte, bufferSize)

		for {
			_, err := file.Read(buffer)
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Println(err)
				break
			}
		}
		output.Write(buffer)
	}

	return output.Bytes(), nil

}
