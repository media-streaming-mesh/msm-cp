package core

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/peer"
	"io"
	"net"
	"strings"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/media-streaming-mesh/msm-cp/internal/rtm"
	"github.com/media-streaming-mesh/msm-cp/internal/stub"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	"github.com/media-streaming-mesh/msm-cp/pkg/stream_api"
)

type API interface {
	Send(conn pb.MsmControlPlane_SendServer) error
}

type Protocol struct {
	cfg         *config.Cfg
	logger      *logrus.Logger
	stubHandler *stub.StubHandler
	rtmImpl     rtm.API
	streamAPI   *stream_api.StreamAPI
}

func New(cfg *config.Cfg) *Protocol {
	return &Protocol{
		cfg:         cfg,
		logger:      cfg.Logger,
		stubHandler: stub.NewStubHandler(cfg),
		rtmImpl:     rtm.New(cfg),
		streamAPI:   stream_api.NewStreamAPI(cfg.Logger),
	}
}

func (p *Protocol) log(format string, args ...interface{}) {
	p.logger.Infof("[GRPC] " + fmt.Sprintf(format, args...))
}

func (p *Protocol) logError(format string, args ...interface{}) {
	p.logger.Errorf("[GRPC] " + fmt.Sprintf(format, args...))
}

func (p *Protocol) Send(conn pb.MsmControlPlane_SendServer) error {
	ctx := conn.Context()
	for {
		// exit if context is done or continue
		select {
		case <-ctx.Done():
			p.log("received connection done")
			return nil
		default:
		}

		// Process stream data
		stream, err := conn.Recv()
		if err == io.EOF {
			// return will close stream-mapper from server side
			p.logError("found EOF, exiting")
			return nil
		}
		if err != nil {
			p.logError("received error %v", err)
			continue
		}

		var streamData *model.StreamData
		connectionKey := model.NewConnectionKey(stream.Local, stream.Remote)

		switch stream.Event {
		case pb.Event_REGISTER:
			p.log("Received REGISTER event: %v", stream)
			// TODO: Find a cleaner way to map node ip for stub
			var proxyIp string
			if stream != nil {
				nodeInfos := strings.Split(stream.Data, ":")
				p.log("node infos %v count %v", nodeInfos, len(nodeInfos))
				if len(nodeInfos) > 0 {
					proxyIp, _ = node_mapper.MapNode(nodeInfos[0])
				}
			}
			if proxyIp == "" {
				contextPeer, _ := peer.FromContext(ctx)
				proxyIp, _, _ = net.SplitHostPort(contextPeer.Addr.String())
			}
			p.stubHandler.OnRegistration(conn, proxyIp)
		case pb.Event_ADD:
			p.log("Received ADD event: %v", stream)
			p.rtmImpl.OnAdd(connectionKey, p.stubHandler.StubChannels)
			p.stubHandler.OnAdd(conn, stream)
		case pb.Event_DELETE:
			p.log("Received DELETE event: %v", stream)
			streamData, err = p.rtmImpl.OnDelete(connectionKey)
			p.stubHandler.OnDelete(connectionKey, conn)
		case pb.Event_DATA:
			p.log("Received DATA event: %v", stream)
			streamData, err = p.rtmImpl.OnData(conn, stream)
		default:
		}

		if err != nil {
			return err
		}

		// Send data to proxy
		if streamData != nil {
			stubAddress := stub.GetStubAddress(streamData.ClientIp, stream.Remote)
			p.log("StubAddress %v", stubAddress)

			streamData := model.StreamData{
				StubIp:      stubAddress,
				ServerIp:    streamData.ServerIp,
				ClientIp:    streamData.ClientIp,
				ServerPorts: streamData.ServerPorts,
				ClientPorts: streamData.ClientPorts,
				StreamState: streamData.StreamState,
			}

			error := p.streamAPI.Put(streamData)

			if error != nil {
				p.logError("Put stream to etcd failed %v", error)
			}
		}
	}
}
