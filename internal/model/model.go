package model

import (
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/media-streaming-mesh/msm-k8s/pkg/model"
)

// ============================================= Stub =======================================
type StubChannel struct {
	Key              model.ConnectionKey
	Request          chan StubChannelRequest
	Response         chan StubChannelResponse
	ReceivedResponse bool
}

func NewStubChannel() StubChannel {
	return StubChannel{
		model.NewConnectionKey("", ""),
		make(chan StubChannelRequest, 1),
		make(chan StubChannelResponse, 1),
		false,
	}
}

type StubChannelRequestType string

const (
	Config StubChannelRequestType = "CONFIG"
	Add    StubChannelRequestType = "ADD"
	Data   StubChannelRequestType = "DATA"
)

type StubChannelRequest struct {
	Type    StubChannelRequestType
	Local   string
	Remote  string
	Request *base.Request
}
type StubChannelResponse struct {
	Error    error
	Response *base.Response
}
