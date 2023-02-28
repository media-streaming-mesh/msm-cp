package stub

import (
	"fmt"
	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/util"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/peer"
	"net"
	"sync"
)

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

type Client struct {
	ClientId string
	ClientIp string
	Port     uint32
}

type StubHandler struct {
	logger *logrus.Logger
}

func NewStubHandler(cfg *config.Cfg) *StubHandler {
	StubMap = new(sync.Map)

	return &StubHandler{
		logger: cfg.Logger,
	}
}

func (s *StubHandler) log(format string, args ...interface{}) {
	s.logger.Infof("[Stub Handler] " + fmt.Sprintf(format, args...))
}

func (s *StubHandler) logError(format string, args ...interface{}) {
	s.logger.Errorf("[Stub Handler] " + fmt.Sprintf(format, args...))
}

func (s *StubHandler) Send(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	switch stream.Event {
	case pb.Event_REGISTER:
		s.onRegistration(conn)
	case pb.Event_ADD:
		s.onAdd(conn, stream)
	case pb.Event_DELETE:
		s.onDelete(conn, stream)
	default:
	}
}

// Call when receive REGISTRATION event
func (s *StubHandler) onRegistration(conn pb.MsmControlPlane_SendServer) {

	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	remoteAddr, _, _ := net.SplitHostPort(p.Addr.String())

	sc := NewStubConnection(remoteAddr, conn)

	//Send node ip address to stub
	dataplaneIP, err := node_mapper.MapNode(remoteAddr)
	s.log("Send msm-proxy ip %v:8050 to %v", dataplaneIP, remoteAddr)
	if err == nil {
		configMsg := &pb.Message{
			Event:  pb.Event_CONFIG,
			Remote: fmt.Sprintf("%s:8050", dataplaneIP),
		}
		sc.Conn.Send(configMsg)
	}

	// save stub connection on a sync.Map
	StubMap.Store(remoteAddr, sc)
	s.log("Connection for client: %s successfully registered", remoteAddr)
}

// Call when receive ADD event
func (s *StubHandler) onAdd(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {

	//Stub send to add channel
	stubAddr := util.GetRemoteIPv4Address(stream.Remote)
	sc, ok := StubMap.Load(stubAddr)
	if ok {
		sc.(*StubConnection).Data = *stream
		sc.(*StubConnection).AddCh <- stream
		sc.(*StubConnection).SendToAddCh = true
		s.log("StubConnection send %v to add channel", stream)
	} else {
		s.onAddExternalClient(conn, stream)
	}
}

func (s *StubHandler) onAddExternalClient(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())
	remoteAddr := util.GetRemoteIPv4Address(stream.Remote)

	sc, ok := StubMap.Load(stubAddr)
	if !ok {
		s.logError("Gateway stub connection was not found! %v", stubAddr)
		return
	}

	//Add clients to stub
	sc.(*StubConnection).Clients[stream.Remote] = Client{
		stream.Remote,
		remoteAddr,
		0,
	}
	s.log("Save client %v with key %v to stub %v", remoteAddr, stream.Remote, stubAddr)
}

// Call when receive DELETE event
func (s *StubHandler) onDelete(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	//Stub unblock add chanel
	stubAddr := util.GetRemoteIPv4Address(stream.Remote)
	sc, ok := StubMap.Load(stubAddr)
	if ok {
		if sc.(*StubConnection).SendToAddCh {
			<-sc.(*StubConnection).AddCh
			sc.(*StubConnection).SendToAddCh = false
		}
	} else {
		s.onDeleteExternalClient(conn, stream)
	}
}

func (s *StubHandler) onDeleteExternalClient(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())
	remoteAddr := util.GetRemoteIPv4Address(stream.Remote)

	sc, ok := StubMap.Load(stubAddr)
	if !ok {
		s.logError("Gateway stub connection was not found! %v", stubAddr)
		return
	}
	delete(sc.(*StubConnection).Clients, stream.Remote)
	s.log("Connection for external client: %s successfully close", remoteAddr)
	s.log("Delete client %v with key %v from stub %v", remoteAddr, stream.Remote, stubAddr)
}

func GetStubAddress(clientIp string, clientId string) string {
	var stubAddress = clientIp
	StubMap.Range(func(key, value interface{}) bool {
		stub := value.(*StubConnection)
		for _, c := range stub.Clients {
			if c.ClientIp == clientIp && c.ClientId == clientId {
				stubAddress = key.(string)
				break
			}
		}
		return true
	})
	return stubAddress
}
