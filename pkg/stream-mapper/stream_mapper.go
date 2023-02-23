package stream_mapper

import (
	"errors"
	"fmt"
	pb_dp "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/media-streaming-mesh/msm-cp/internal/transport"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	"github.com/sirupsen/logrus"
	"sync"
)

type StreamMapper struct {
	logger    *logrus.Logger
	streamMap *sync.Map
}

func NewStreamMapper(logger *logrus.Logger, streamMap *sync.Map) *StreamMapper {
	return &StreamMapper{
		logger:    logger,
		streamMap: streamMap,
	}
}

func (m *StreamMapper) log(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	m.logger.Debugf("[Stream Mapper] " + fmt.Sprintf(format, args...))
}

func (m *StreamMapper) ProcessStream(data model.StreamData) error {
	var serverProxyIP string
	var clientProxyIP string
	var serverDpGrpcClient transport.Client
	var clientDpGrpcClient transport.Client

	m.log("Processing stream %v", data)

	if len(data.ServerPorts) == 0 || len(data.ClientPorts) == 0 {
		return errors.New("[Stream Mapper] empty server/client port")
	}

	//Check if client/server on same node
	clientIp := data.StubIp
	if clientIp == "" {
		clientIp = data.ClientIp
	}
	isOnSameNode := node_mapper.IsOnSameNode(clientIp, data.ServerIp)
	m.log("server %v and client %v - same node is %v", data.ServerIp, clientIp, isOnSameNode)

	//Get proxy ip
	clientProxyIP, err := node_mapper.MapNode(clientIp)
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

	//Create GRPC connection
	clientGrpcClient, err := transport.SetupClient(clientProxyIP)
	if err != nil {
		m.log("Failed to setup GRPC client, error %s\n", err)
	}
	clientDpGrpcClient = transport.Client{
		m.logger,
		clientGrpcClient,
	}

	if !isOnSameNode {
		serverGrpcClient, err := transport.SetupClient(serverProxyIP)
		if err != nil {
			m.log("Failed to setup GRPC client, error %s\n", err)
		}
		serverDpGrpcClient = transport.Client{
			m.logger,
			serverGrpcClient,
		}
	}

	//Send data to proxy
	if data.StreamState == model.Create {
		var stream model.Stream
		savedStream, _ := m.streamMap.Load(data.ServerIp)

		//Send Create Stream to proxy
		if savedStream == nil {
			streamId := model.GetStreamID()
			m.log("stream Id %v", streamId)

			//Create new stream
			stream = model.Stream{
				StreamId: streamId,
				ProxyMap: make(map[string]model.Proxy),
			}

			if isOnSameNode {
				// add the stream-mapper to the proxy
				streamData, result := clientDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, data.ServerIp, data.ServerPorts[0])
				m.log("Create stream %v result %v", streamData, *result)
			} else {
				// add the stream-mapper to the server proxy
				streamData, result := serverDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, data.ServerIp, data.ServerPorts[0])
				m.log("Create server stream %v result %v", streamData, *result)

				// add the stream-mapper to the client proxy
				streamData, result = clientDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, serverProxyIP, data.ServerPorts[0])
				m.log("Create client stream %v result %v", streamData, *result)

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
				m.log("Create client stream %v result %v", streamData, *result)

				// add the client proxy to the proxy map
				stream.ProxyMap[clientProxyIP] = model.Proxy{
					ProxyIp:     clientProxyIP,
					StreamState: model.Create,
				}
			}
		}

		//Send Create Endpoint to proxy
		clientProxy := stream.ProxyMap[clientProxyIP]
		m.log("Client proxy %v total clients %v", clientProxy, len(clientProxy.Clients))

		if !isOnSameNode && clientProxy.StreamState < model.Play {
			endpoint, result := serverDpGrpcClient.CreateEndpoint(stream.StreamId, pb_dp.Encap_RTP_UDP, clientProxyIP, 8050)
			m.log("Created proxy-proxy ep %v result %v", endpoint, *result)
		}

		endpoint, result := clientDpGrpcClient.CreateEndpoint(stream.StreamId, pb_dp.Encap_RTP_UDP, data.ClientIp, data.ClientPorts[0])
		m.log("Create client ep %v result %v", endpoint, result)

		//Update streamMap data
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
				m.log("Update ep %v result %v", endpoint, result)

				if !isOnSameNode && clientProxy.StreamState < model.Play {
					endpoint2, result := serverDpGrpcClient.UpdateEndpoint(stream.StreamId, clientProxyIP, 8050)
					stream.ProxyMap[clientProxyIP] = model.Proxy{
						ProxyIp:     clientProxy.ProxyIp,
						StreamState: model.Play,
						Clients:     clientProxy.Clients,
					}
					m.log("Update proxy ep %v result %v", endpoint2, result)
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
				m.log("Delete ep %v %v", endpoint, result)
				clientProxy.Clients = append(clientProxy.Clients[:i], clientProxy.Clients[i+1:]...)
				break
			}
		}

		stream.ProxyMap[clientProxyIP] = model.Proxy{
			ProxyIp:     clientProxy.ProxyIp,
			StreamState: clientProxy.StreamState,
			Clients:     clientProxy.Clients,
		}

		//End proxy-proxy connection
		if !isOnSameNode && clientProxy.StreamState < model.Teardown && len(clientProxy.Clients) == 0 {
			endpoint2, result := serverDpGrpcClient.DeleteEndpoint(stream.StreamId, clientProxyIP, 8050)
			delete(stream.ProxyMap, clientProxyIP)
			m.log("Delete ep %v %v", endpoint2, result)
		}

		//Delete stream if all clients terminate
		if m.getClientCount(data.ServerIp) == 0 {
			if isOnSameNode {
				streamData, result := clientDpGrpcClient.DeleteStream(stream.StreamId, data.ServerIp, 8050)
				m.streamMap.Delete(data.ServerIp)
				m.log("Delete stream-mapper %v %v", streamData, result)
			} else {
				streamData, result := serverDpGrpcClient.DeleteStream(stream.StreamId, data.ServerIp, 8050)
				m.streamMap.Delete(data.ServerIp)
				m.log("Delete stream-mapper %v %v", streamData, result)

				streamData2, result := clientDpGrpcClient.DeleteStream(stream.StreamId, serverProxyIP, 8050)
				m.log("Delete stream-mapper %v %v", streamData2, result)
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
