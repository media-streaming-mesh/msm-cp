package stub

import (
	"fmt"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/media-streaming-mesh/msm-cp/internal/rtm"
	"github.com/media-streaming-mesh/msm-cp/internal/util"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	stream_mapper "github.com/media-streaming-mesh/msm-cp/pkg/stream-mapper"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/peer"
	"io"
	"net"
	"sync"
)

// API provides external access to stub
type StubAPI interface {
	Send(conn pb.MsmControlPlane_SendServer) error
}

type StubHandler struct {
	logger  *logrus.Logger
	rtmImpl rtm.API

	//TODO: move stream_mapper to msm-nc
	streamMapper *stream_mapper.StreamMapper
}

func NewStubHandler(cfg *config.Cfg) *StubHandler {
	model.StubMap = new(sync.Map)
	protocol := rtm.New(cfg)

	return &StubHandler{
		logger:       cfg.Logger,
		rtmImpl:      protocol,
		streamMapper: stream_mapper.NewStreamMapper(cfg.Logger, new(sync.Map)),
	}
}

func (s *StubHandler) log(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	s.logger.Debugf("[Stub Handler] " + fmt.Sprintf(format, args...))
}

func (s *StubHandler) Send(conn pb.MsmControlPlane_SendServer) error {
	var ctx = conn.Context()
	for {
		// exit if context is done or continue
		select {
		case <-ctx.Done():
			s.log("reveiced connection done")
			return ctx.Err()
		default:
		}

		//Process stream data
		stream, err := conn.Recv()
		if err == io.EOF {
			// return will close stream-mapper from server side
			s.log("found EOF, exiting")
			return nil
		}
		if err != nil {
			s.log("received error %v", err)
			continue
		}

		switch stream.Event {
		case pb.Event_REGISTER:
			s.log("Received REGISTER event: %v", stream)
			s.OnRegistration(conn)

		case pb.Event_ADD:
			s.log("Received ADD event: %v", stream)
			s.OnAdd(conn, stream)

		case pb.Event_DELETE:
			s.log("Received DELETE event: %v", stream)
			s.OnDelete(conn, stream)

		case pb.Event_DATA:
			s.log("Received DATA event: %v", stream)
			s.OnData(conn, stream)
		default:
		}
	}
}

// Call when receive REGISTRATION event
func (s *StubHandler) OnRegistration(conn pb.MsmControlPlane_SendServer) {

	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	remoteAddr, _, _ := net.SplitHostPort(p.Addr.String())

	sc := model.NewStubConnection(remoteAddr, conn)

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
	model.StubMap.Store(remoteAddr, sc)
	s.log("Connection for client: %s successfully registered", remoteAddr)
}

// Call when receive ADD event
func (s *StubHandler) OnAdd(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {

	//Stub send to add channel
	stubAddr := util.GetRemoteIPv4Address(stream.Remote)
	sc, ok := model.StubMap.Load(stubAddr)
	if ok {
		//Add RTSP connection
		s.rtmImpl.OnAdd(stream)

		sc.(*model.StubConnection).Data = *stream
		sc.(*model.StubConnection).AddCh <- stream
		sc.(*model.StubConnection).SendToAddCh = true
		s.log("StubConnection send %v to add channel", stream)
	} else {
		s.OnAddExternalClient(conn, stream)
	}
}

func (s *StubHandler) OnAddExternalClient(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())
	remoteAddr := util.GetRemoteIPv4Address(stream.Remote)

	sc, ok := model.StubMap.Load(stubAddr)
	if !ok {
		s.logger.Errorf("[Stub Handler] Gateway stub connection was not found! %v", stubAddr)
		return
	}

	//Add RTSP connection
	s.rtmImpl.OnAddExternalClient(stream, sc.(*model.StubConnection).Data)

	//Add clients to stub
	sc.(*model.StubConnection).Clients[stream.Remote] = model.Client{
		stream.Remote,
		remoteAddr,
		0,
	}
	s.log("Save client %v with key %v to stub %v", remoteAddr, stream.Remote, stubAddr)
}

// Call when receive DELETE event
func (s *StubHandler) OnDelete(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	//Delete RTSP connection
	streamData := s.rtmImpl.OnDelete(stream)

	//Stub unblock add chanel
	stubAddr := util.GetRemoteIPv4Address(stream.Remote)
	sc, ok := model.StubMap.Load(stubAddr)
	if ok {
		if sc.(*model.StubConnection).SendToAddCh {
			<-sc.(*model.StubConnection).AddCh
			sc.(*model.StubConnection).SendToAddCh = false
		}
	} else {
		s.OnDeleteExternalClient(conn, stream)
	}

	//Send delete to proxy
	if streamData != nil {
		stubAddress := s.getStubAddress(streamData.ClientIp, stream.Remote)

		//TODO: add stub address to stream data and send to stream mapper
		error := s.streamMapper.ProcessStream(model.StreamData{
			StubIp:      stubAddress,
			ServerIp:    streamData.ServerIp,
			ClientIp:    streamData.ClientIp,
			ServerPorts: streamData.ServerPorts,
			ClientPorts: streamData.ClientPorts,
			StreamState: streamData.StreamState,
		})

		if error != nil {
			s.log("ProcessStream failed %v", error)
		}
	}
}

func (s *StubHandler) OnDeleteExternalClient(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())
	remoteAddr := util.GetRemoteIPv4Address(stream.Remote)

	sc, ok := model.StubMap.Load(stubAddr)
	if !ok {
		s.logger.Errorf("[Stub Handler] Gateway stub connection was not found! %v", stubAddr)
		return
	}
	delete(sc.(*model.StubConnection).Clients, stream.Remote)
	s.log("Connection for external client: %s successfully close", remoteAddr)
	s.log("Delete client %v with key %v from stub %v", remoteAddr, stream.Remote, stubAddr)
}

// Call when receive DATA event
func (s *StubHandler) OnData(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	//Process RTSP connection
	streamData := s.rtmImpl.OnData(conn, stream)

	s.log("Stream  %v", stream)
	s.log("Stream data %v", streamData)

	if streamData != nil {
		stubAddress := s.getStubAddress(streamData.ClientIp, stream.Remote)
		s.log("StubAddress %v", stubAddress)

		//TODO: add stub address to stream data and send to stream mapper
		error := s.streamMapper.ProcessStream(model.StreamData{
			StubIp:      stubAddress,
			ServerIp:    streamData.ServerIp,
			ClientIp:    streamData.ClientIp,
			ServerPorts: streamData.ServerPorts,
			ClientPorts: streamData.ClientPorts,
			StreamState: streamData.StreamState,
		})

		if error != nil {
			s.log("ProcessStream failed %v", error)
		}
	}
}

func (s *StubHandler) getStubAddress(clientIp string, clientId string) string {
	var stubAddress = ""
	model.StubMap.Range(func(key, value interface{}) bool {
		stub := value.(*model.StubConnection)
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
