package stream_mapper

import (
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"

	pb_dp "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/media-streaming-mesh/msm-cp/internal/transport"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	"github.com/media-streaming-mesh/msm-cp/pkg/stream_api"
)

var streamId StreamId

// Stream id type uint32 auto increment
type StreamId struct {
	sync.Mutex
	id uint32
}

type StreamMapper struct {
	logger    *logrus.Logger
	dataChan  chan model.StreamData
	streamMap *sync.Map
	streamAPI *stream_api.StreamAPI
}

func NewStreamMapper(logger *logrus.Logger, streamMap *sync.Map) *StreamMapper {
	return &StreamMapper{
		logger:    logger,
		dataChan:  make(chan model.StreamData, 1),
		streamMap: streamMap,
		streamAPI: stream_api.NewStreamAPI(logger),
	}
}

func (m *StreamMapper) log(format string, args ...interface{}) {
	m.logger.Infof("[Stream Mapper] " + fmt.Sprintf(format, args...))
}

func (m *StreamMapper) logError(format string, args ...interface{}) {
	m.logger.Errorf("[Stream Mapper] " + fmt.Sprintf(format, args...))
}

func (m *StreamMapper) WatchStream() {
	//TODO: process previous cached streams
	m.streamAPI.GetStreams()

	m.waitForData()
	m.streamAPI.WatchStreams(m.dataChan)
}

func (m *StreamMapper) waitForData() {
	go func() {
		streamData := <-m.dataChan
		error := m.processStream(streamData)
		if error != nil {
			m.logError("Process stream failed %v", error)
		}
		m.waitForData()
	}()
}

func (m *StreamMapper) processStream(data model.StreamData) error {
	var serverProxyIP string
	var clientProxyIP string
	var serverDpGrpcClient transport.Client
	var clientDpGrpcClient transport.Client

	m.log("Processing stream %v", data)

	if len(data.ServerPorts) == 0 || len(data.ClientPorts) == 0 {
		return errors.New("[Stream Mapper] empty server/client port")
	}

	// Check if client/server on same node
	isOnSameNode := node_mapper.IsOnSameNode(data.StubIp, data.ServerIp)
	m.log("server %v and client %v - same node is %v", data.ServerIp, data.StubIp, isOnSameNode)

	// Get proxy ip
	clientProxyIP, err := node_mapper.MapNode(data.StubIp)
	if err != nil {
		return err
	}
	m.log("client msm-proxy ip %v", clientProxyIP)
	if !isOnSameNode {
		serverProxyIP, err = node_mapper.MapNode(data.ServerIp)
		if err != nil {
			return err
		}
		m.log("server msm-proxy ip %v", serverProxyIP)
	}

	// Create GRPC connection
	clientGrpcClient, err := transport.SetupClient(clientProxyIP)
	if err != nil {
		m.logError("Failed to setup GRPC client, error %s\n", err)
	}
	clientDpGrpcClient = transport.Client{
		m.logger,
		clientGrpcClient,
	}

	if !isOnSameNode {
		serverGrpcClient, err := transport.SetupClient(serverProxyIP)
		if err != nil {
			m.logError("Failed to setup GRPC client, error %s\n", err)
		}
		serverDpGrpcClient = transport.Client{
			m.logger,
			serverGrpcClient,
		}
	}

	// Send data to proxy
	if data.StreamState == model.Create {
		var stream model.Stream
		savedStream, _ := m.streamMap.Load(data.ServerIp)

		// Send Create Stream to proxy
		if savedStream == nil {
			streamId := GetStreamID()
			m.log("stream Id %v", streamId)

			// Create new stream
			stream = model.Stream{
				StreamId: streamId,
				ProxyMap: make(map[string]model.Proxy),
			}

			if isOnSameNode {
				// add the stream-mapper to the proxy
				streamData, result := clientDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, data.ServerIp, data.ServerPorts[0])
				m.log("GRPC create stream %v result %v", streamData, *result)
			} else {
				// add the stream-mapper to the server proxy
				streamData, result := serverDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, data.ServerIp, data.ServerPorts[0])
				m.log("GRPC create server stream %v result %v", streamData, *result)

				// add the stream-mapper to the client proxy
				streamData, result = clientDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, serverProxyIP, data.ServerPorts[0])
				m.log("GRPC create client stream %v result %v", streamData, *result)

				// add the server proxy to the proxy map
				stream.ProxyMap[serverProxyIP] = model.Proxy{
					ProxyIp:     serverProxyIP,
					StreamState: model.Create,
				}
			}

			// add the client proxy to the proxy map
			stream.ProxyMap[clientProxyIP] = model.Proxy{
				ProxyIp:     clientProxyIP,
				StreamState: model.Create,
			}

			// now store the RTSP stream-mapper
			m.streamMap.Store(data.ServerIp, stream)
		} else {
			stream = savedStream.(model.Stream)
			if _, exists := stream.ProxyMap[clientProxyIP]; !exists {
				// add the stream-mapper to the client proxy
				streamData, result := clientDpGrpcClient.CreateStream(stream.StreamId, pb_dp.Encap_RTP_UDP, serverProxyIP, data.ServerPorts[0])
				m.log("GRPC create client stream %v result %v", streamData, *result)

				// add the client proxy to the proxy map
				stream.ProxyMap[clientProxyIP] = model.Proxy{
					ProxyIp:     clientProxyIP,
					StreamState: model.Create,
				}
			}
		}

		// Send Create Endpoint to proxy
		clientProxy := stream.ProxyMap[clientProxyIP]
		m.log("Client proxy %v total clients %v", clientProxy, len(clientProxy.Clients))

		if !isOnSameNode && clientProxy.StreamState < model.Play {
			endpoint, result := serverDpGrpcClient.CreateEndpoint(stream.StreamId, pb_dp.Encap_RTP_UDP, clientProxyIP, 8050)
			m.log("GRPC created proxy-proxy ep %v result %v", endpoint, *result)
		}

		endpoint, result := clientDpGrpcClient.CreateEndpoint(stream.StreamId, pb_dp.Encap_RTP_UDP, data.ClientIp, data.ClientPorts[0])
		m.log("GRPC create client ep %v result %v", endpoint, result)

		// Update streamMap data
		clientProxy.Clients = append(clientProxy.Clients, model.Client{
			ClientIp: data.ClientIp,
			Port:     data.ClientPorts[0],
		})
		stream.ProxyMap[clientProxyIP] = model.Proxy{
			ProxyIp:     clientProxyIP,
			StreamState: clientProxy.StreamState,
			Clients:     clientProxy.Clients,
		}
		m.log("Stream %v", stream)
	}

	if data.StreamState == model.Play {
		savedStream, _ := m.streamMap.Load(data.ServerIp)
		if savedStream == nil {
			return errors.New("[Stream Mapper] Can't find stream")
		}

		stream := savedStream.(model.Stream)
		clientProxy, exists := stream.ProxyMap[clientProxyIP]
		if !exists {
			return errors.New("[Stream Mapper] Can't find client proxy")
		}
		m.log("Client proxy PLAY proxy clients %v total clients %v", clientProxy, len(clientProxy.Clients))
		for _, c := range clientProxy.Clients {
			if c.ClientIp == data.ClientIp && c.Port == data.ClientPorts[0] {
				endpoint, result := clientDpGrpcClient.UpdateEndpoint(stream.StreamId, data.ClientIp, data.ClientPorts[0])
				m.log("GRPC update ep %v result %v", endpoint, result)

				if !isOnSameNode && clientProxy.StreamState < model.Play {
					endpoint2, result := serverDpGrpcClient.UpdateEndpoint(stream.StreamId, clientProxyIP, 8050)
					stream.ProxyMap[clientProxyIP] = model.Proxy{
						ProxyIp:     clientProxy.ProxyIp,
						StreamState: model.Play,
						Clients:     clientProxy.Clients,
					}
					m.log("GRPC update proxy ep %v result %v", endpoint2, result)
				}
				break
			}
		}
	}

	if data.StreamState == model.Teardown {
		savedStream, _ := m.streamMap.Load(data.ServerIp)
		if savedStream == nil {
			return errors.New("[Stream Mapper] Can't find stream")
		}

		stream := savedStream.(model.Stream)
		clientProxy, exists := stream.ProxyMap[clientProxyIP]
		if !exists {
			return errors.New("[Stream Mapper] Can't find client proxy")
		}
		m.log("Client proxy TEARDOWN %v proxy clients %v total clients %v", clientProxy, len(clientProxy.Clients), m.getClientCount(data.ServerIp))

		for i, c := range clientProxy.Clients {
			if c.ClientIp == data.ClientIp && c.Port == data.ClientPorts[0] {
				endpoint, result := clientDpGrpcClient.DeleteEndpoint(stream.StreamId, data.ClientIp, data.ClientPorts[0])
				m.log("GRPC delete ep %v %v", endpoint, result)
				clientProxy.Clients = append(clientProxy.Clients[:i], clientProxy.Clients[i+1:]...)
				break
			}
		}

		stream.ProxyMap[clientProxyIP] = model.Proxy{
			ProxyIp:     clientProxy.ProxyIp,
			StreamState: clientProxy.StreamState,
			Clients:     clientProxy.Clients,
		}

		// End proxy-proxy connection
		if !isOnSameNode && clientProxy.StreamState < model.Teardown && len(clientProxy.Clients) == 0 {
			endpoint2, result := serverDpGrpcClient.DeleteEndpoint(stream.StreamId, clientProxyIP, 8050)
			delete(stream.ProxyMap, clientProxyIP)
			m.log("GRPC delete ep %v %v", endpoint2, result)
		}

		// Delete stream if all clients terminate
		if m.getClientCount(data.ServerIp) == 0 {
			if isOnSameNode {
				streamData, result := clientDpGrpcClient.DeleteStream(stream.StreamId, data.ServerIp, 8050)
				m.streamMap.Delete(data.ServerIp)
				m.log("GRPC delete stream %v %v", streamData, result)
			} else {
				streamData, result := serverDpGrpcClient.DeleteStream(stream.StreamId, data.ServerIp, 8050)
				m.streamMap.Delete(data.ServerIp)
				m.log("GRPC delete stream %v %v", streamData, result)

				streamData2, result := clientDpGrpcClient.DeleteStream(stream.StreamId, serverProxyIP, 8050)
				m.log("GRPC delete stream %v %v", streamData2, result)
			}
		}
	}

	clientDpGrpcClient.Close()
	if !isOnSameNode {
		serverDpGrpcClient.Close()
	}
	return nil
}

func (m *StreamMapper) getClientCount(serverEp string) int {
	count := 0
	data, _ := m.streamMap.Load(serverEp)
	if data != nil {
		for _, v := range data.(model.Stream).ProxyMap {
			count += len(v.Clients)
		}
	}
	return count
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
