package stub

import (
	"bytes"
	"fmt"
	"net"
	"sync"

	"github.com/aler9/gortsplib/pkg/base"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/peer"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/media-streaming-mesh/msm-cp/internal/util"
	"time"
)

var StubMap *sync.Map

type StubConnection struct {
	Address string
	Conn    pb.MsmControlPlane_SendServer
	Clients map[string]Client
}

func NewStubConnection(address string, conn pb.MsmControlPlane_SendServer) *StubConnection {
	return &StubConnection{
		Address: address,
		Conn:    conn,
		Clients: make(map[string]Client),
	}
}

type Client struct {
	ClientId string
	ClientIp string
	Port     uint32
}

type StubHandler struct {
	logger       *logrus.Logger
	StubChannels map[string]*model.StubChannel
}

func NewStubHandler(cfg *config.Cfg) *StubHandler {
	StubMap = new(sync.Map)

	return &StubHandler{
		logger:       cfg.Logger,
		StubChannels: make(map[string]*model.StubChannel),
	}
}

func (s *StubHandler) log(format string, args ...interface{}) {
	s.logger.Infof("[Stub Handler] " + fmt.Sprintf(format, args...))
}

func (s *StubHandler) logError(format string, args ...interface{}) {
	s.logger.Errorf("[Stub Handler] " + fmt.Sprintf(format, args...))
}

// Call when receive REGISTRATION event
func (s *StubHandler) OnRegistration(conn pb.MsmControlPlane_SendServer, proxyIp string) {
	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	remoteAddr, _, _ := net.SplitHostPort(p.Addr.String())

	//save stub connection on a sync.Map
	sc := NewStubConnection(remoteAddr, conn)

	StubMap.Store(remoteAddr, sc)
	s.log("Connection for client: %s successfully registered", remoteAddr)

	//create channels
	channel := model.NewStubChannel()
	s.StubChannels[remoteAddr] = &channel
	s.log("Add channels to %v", remoteAddr)

	//wait for request
	s.waitForRequest(remoteAddr)

	//send config
	channel.Request <- model.StubChannelRequest{
		model.Config,
		"",
		proxyIp,
		nil,
	}
}

// Call when receive ADD event
func (s *StubHandler) OnAdd(conn pb.MsmControlPlane_SendServer, stream *pb.Message) {
	// Stub send to add channel
	stubAddr := util.GetRemoteIPv4Address(stream.Remote)
	_, ok := StubMap.Load(stubAddr)
	if ok {
		connectionKey := model.NewConnectionKey(stream.Local, stream.Remote)
		stubChannel, ok := s.StubChannels[stubAddr]
		if ok {
			stubChannel.Key = model.NewConnectionKey(stream.Local, stream.Remote)
			stubChannel.Response <- model.StubChannelResponse{
				nil,
				&base.Response{
					StatusCode: base.StatusOK,
					Header:     base.Header{},
				},
			}
			stubChannel.ReceivedResponse = true
		} else {
			s.logError("Can't find stub channel for %v", stubAddr)
		}
		s.log("send %v to receiveAdd channel", connectionKey)
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

	// Add clients to stub
	sc.(*StubConnection).Clients[stream.Remote] = Client{
		stream.Remote,
		remoteAddr,
		0,
	}
	s.log("Save client %v with key %v to stub %v", remoteAddr, stream.Remote, stubAddr)
}

// Call when receive DELETE event
func (s *StubHandler) OnDelete(connectionKey model.ConnectionKey, conn pb.MsmControlPlane_SendServer) {
	//Stub unblock add chanel
	stubAddr := util.GetRemoteIPv4Address(connectionKey.Remote)
	_, ok := StubMap.Load(stubAddr)
	if !ok {
		s.onDeleteExternalClient(connectionKey, conn)
	}
}

func (s *StubHandler) onDeleteExternalClient(connectionKey model.ConnectionKey, conn pb.MsmControlPlane_SendServer) {
	// get remote ip addr
	ctx := conn.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())
	remoteAddr := util.GetRemoteIPv4Address(connectionKey.Remote)

	sc, ok := StubMap.Load(stubAddr)
	if !ok {
		s.logError("Gateway stub connection was not found! %v", stubAddr)
		return
	}
	delete(sc.(*StubConnection).Clients, connectionKey.Remote)
	s.log("Connection for external client: %s successfully close", remoteAddr)
	s.log("Delete client %v with key %v from stub %v", remoteAddr, connectionKey.Remote, stubAddr)
}

func GetStubAddress(clientIp string, clientId string) string {
	stubAddress := clientIp
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

func (s *StubHandler) waitForRequest(key string) {
	channel := s.StubChannels[key]
	go func() {
		request := <-channel.Request
		timeout := request.Type != model.Config
		s.log("Processing request %v", request.Type)
		s.sendRequest(channel, key, request, timeout)
	}()
}

func (s *StubHandler) sendRequest(channel *model.StubChannel, key string, request model.StubChannelRequest, timeout bool) {
	stubConn, ok := StubMap.Load(key)
	if !ok {
		s.logError("could not find stub connection for %v", key)
	}

	var msg *pb.Message
	switch request.Type {
	case model.Config:
		msg = &pb.Message{
			Event:  pb.Event_CONFIG,
			Remote: fmt.Sprintf("%s:8050", request.Remote),
		}
	case model.Add:
		msg = &pb.Message{
			Event:  pb.Event_REQUEST,
			Remote: request.Remote,
		}
	case model.Data:
		data := bytes.NewBuffer(make([]byte, 0, 4096))
		request.Request.Write(data)
		msg = &pb.Message{
			Event:  pb.Event_DATA,
			Local:  request.Local,
			Remote: request.Remote,
			Data:   fmt.Sprintf("%s", data),
		}
	}
	stubConn.(*StubConnection).Conn.Send(msg)
	s.log("Send %v with %v to %v", request.Type, msg, key)

	//start wait for request again
	s.waitForRequest(key)

	channel.ReceivedResponse = false
	if timeout {
		time.Sleep(time.Second * 15)
		if channel.ReceivedResponse == false {
			s.log("Timeout request %v %v", request.Type, request.Request)
			s.logError("Send %v to %v timeout", request.Type, key)
			channel.Response <- model.StubChannelResponse{
				fmt.Errorf("Stub request timeout"),
				&base.Response{
					StatusCode: base.StatusRequestTimeout,
					Header:     base.Header{},
				},
			}
			channel.ReceivedResponse = true
		}
	}
}
