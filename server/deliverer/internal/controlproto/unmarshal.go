package controlproto

import "github.com/THPTUHA/kairos/server/deliverer/internal/controlpb"

type Decoder interface {
	DecodeCommand([]byte) (*controlpb.Command, error)
}

var _ Decoder = (*ProtobufDecoder)(nil)

type ProtobufDecoder struct{}

func NewProtobufDecoder() *ProtobufDecoder {
	return &ProtobufDecoder{}
}

func (e *ProtobufDecoder) DecodeCommand(data []byte) (*controlpb.Command, error) {
	var cmd controlpb.Command
	err := cmd.UnmarshalVT(data)
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}
