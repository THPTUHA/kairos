package main

import (
	"fmt"
	"net/http"

	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

type Simple struct {
}

func (s *Simple) Execute(args *proto.ExecuteRequest, cb kplugin.StatusHelper) (*proto.ExecuteResponse, error) {
	out, err := s.ExecuteImpl(args, cb)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Hello, this is a simple HTTP server!")
}

func (s *Simple) ExecuteImpl(args *proto.ExecuteRequest, cb kplugin.StatusHelper) ([]byte, error) {
	http.HandleFunc("/", handler)

	// Start the HTTP server on port 8080
	err := http.ListenAndServe(fmt.Sprintf(":%d", 8090), nil)
	if err != nil {
		fmt.Println("Error:", err)
	}
	fmt.Println("Server is running on http://localhost:8080")

	return nil, nil
}
