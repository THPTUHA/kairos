package controlproto

import "github.com/THPTUHA/kairos/server/deliverer/internal/controlpb"

type Encoder interface {
	EncodeCommand(*controlpb.Command) ([]byte, error)
}

var _ Encoder = (*ProtobufEncoder)(nil)

type ProtobufEncoder struct{}

func NewProtobufEncoder() *ProtobufEncoder {
	return &ProtobufEncoder{}
}

func (e *ProtobufEncoder) EncodeCommand(cmd *controlpb.Command) ([]byte, error) {
	return cmd.MarshalVT()
}
