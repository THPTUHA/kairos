package survey

import (
	"context"

	"github.com/centrifugal/centrifuge"
)

// Handler can handle survey.
type Handler func(node *centrifuge.Node, data []byte) centrifuge.SurveyReply

type Caller struct {
	node     *centrifuge.Node
	handlers map[string]Handler
}

func (c *Caller) Channels(ctx context.Context, cmd *apiproto.ChannelsRequest) (map[string]*apiproto.ChannelInfo, error) {
	return surveyChannels(ctx, c.node, cmd)
}

func NewCaller(node *centrifuge.Node) *Caller {
	c := &Caller{
		node: node,
		handlers: map[string]Handler{
			"channels": respondChannelsSurvey,
		},
	}
	c.node.OnSurvey(func(event centrifuge.SurveyEvent, cb centrifuge.SurveyCallback) {
		h, ok := c.handlers[event.Op]
		if !ok {
			cb(centrifuge.SurveyReply{Code: MethodNotFound})
			return
		}
		cb(h(c.node, event.Data))
	})
	return c
}
