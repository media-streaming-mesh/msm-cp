package model

import (
	"fmt"
	"github.com/aler9/gortsplib/pkg/base"
)

// ============================================= Stub =======================================
type StubChannel struct {
	Key              ConnectionKey
	Request          chan StubChannelRequest
	Response         chan StubChannelResponse
	ReceivedResponse bool
}

func NewStubChannel() StubChannel {
	return StubChannel{
		NewConnectionKey("", ""),
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

//============================================= Connection Key =======================================

type ConnectionKey struct {
	Local  string
	Remote string
	Key    string
}

func NewConnectionKey(local string, remote string) ConnectionKey {
	return ConnectionKey{
		local,
		remote,
		fmt.Sprintf("%s%s", local, remote),
	}
}

//============================================= Stream =============================================

type Stream struct {
	StreamId uint32
	ProxyMap map[string]Proxy
}

type StreamData struct {
	StubIp      string
	ServerIp    string
	ClientIp    string
	ServerPorts []uint32
	ClientPorts []uint32
	StreamState StreamState
}

type StreamState int

const (
	Create   StreamState = 0
	Play                 = 1
	Teardown             = 2
)

// ============================================= Proxy =============================================
type Proxy struct {
	ProxyIp     string
	StreamState StreamState
	Clients     []Client
}

type Client struct {
	ClientId string
	ClientIp string
	Port     uint32
}
