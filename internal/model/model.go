package model

import (
	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"sync"
)

// ============================================= Stub =============================================
var (
	StubMap *sync.Map
)

type StubConnection struct {
	Address     string
	Conn        pb.MsmControlPlane_SendServer
	Data        pb.Message
	SendToAddCh bool
	AddCh       chan *pb.Message
	DataCh      chan *base.Response
	Clients     map[string]Client
}

func NewStubConnection(address string, conn pb.MsmControlPlane_SendServer) *StubConnection {
	return &StubConnection{
		Address: address,
		Conn:    conn,
		AddCh:   make(chan *pb.Message, 1),
		DataCh:  make(chan *base.Response, 1),
		Clients: make(map[string]Client),
	}
}

// ============================================= StreamId =============================================
var streamId StreamId

// Stream id type uint32 auto increment
type StreamId struct {
	sync.Mutex
	id uint32
}

func (si *StreamId) ID() (id uint32) {
	si.Lock()
	defer si.Unlock()
	id = si.id
	si.id++
	return
}

func GetStreamID() uint32 {
	return streamId.ID()
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
