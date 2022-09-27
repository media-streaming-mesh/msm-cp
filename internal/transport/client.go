package transport

import (
	"context"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/sirupsen/logrus"
	"strconv"
	"sync"
)

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

// Client holds the client specific data structures
type Client struct {
	Log        *logrus.Logger
	GrpcClient *grpcClient
}

func SetupClient(ip string) (*grpcClient, error) {
	grpcClient, err := newGRPCClient(ip)
	grpcClient.start()
	return grpcClient, err
}

func (c *Client) Close() {
	c.GrpcClient.close()
}

func (c *Client) CreateStream(ip string, port uint32) (pb.StreamData, *pb.StreamResult) {
	//Prepare CREATE data
	encap, _ := strconv.ParseUint(pb.Encap_RTP_UDP.String(), 10, 32)

	endpoint := pb.Endpoint{
		Ip:    ip,
		Port:  port,
		Encap: uint32(encap),
	}
	req := pb.StreamData{
		Id:        streamId.ID(),
		Operation: pb.StreamOperation_CREATE,
		Protocol:  pb.ProxyProtocol_RTP,
		Endpoint:  &endpoint,
	}

	//Send data to RTPProxy
	stream, _ := c.GrpcClient.client.StreamAddDel(context.Background(), &req)
	return req, stream
}

func (c *Client) CreateEndpoint(streamId uint32, ip string, port uint32) (pb.Endpoint, *pb.StreamResult) {
	//Prepare AddEndpoint data
	encap, _ := strconv.ParseUint(pb.Encap_RTP_UDP.String(), 10, 32)

	endpoint := pb.Endpoint{
		Ip:    ip,
		Port:  port,
		Encap: uint32(encap),
	}
	req := pb.StreamData{
		Id:        streamId,
		Operation: pb.StreamOperation_ADD_EP,
		Protocol:  pb.ProxyProtocol_RTP,
		Endpoint:  &endpoint,
	}

	//Send data to RTPProxy
	stream, _ := c.GrpcClient.client.StreamAddDel(context.Background(), &req)
	return endpoint, stream
}

func (c *Client) UpdateEndpoint(streamId uint32, ip string, port uint32) (pb.Endpoint, *pb.StreamResult) {
	//Prepare AddEndpoint data

	endpoint := pb.Endpoint{
		Ip:   ip,
		Port: port,
	}
	req := pb.StreamData{
		Id:        streamId,
		Operation: pb.StreamOperation_UPD_EP,
		Protocol:  pb.ProxyProtocol_RTP,
		Endpoint:  &endpoint,
		Enable:    true,
	}

	//Send data to RTPProxy
	stream, _ := c.GrpcClient.client.StreamAddDel(context.Background(), &req)
	return endpoint, stream
}

func (c *Client) DeleteEndpoint(streamId uint32, ip string, port uint32) (pb.Endpoint, *pb.StreamResult) {
	//Prepare AddEndpoint data

	endpoint := pb.Endpoint{
		Ip:   ip,
		Port: port,
	}
	req := pb.StreamData{
		Id:        streamId,
		Operation: pb.StreamOperation_DEL_EP,
		Protocol:  pb.ProxyProtocol_RTP,
		Endpoint:  &endpoint,
	}

	//Send data to RTPProxy
	stream, _ := c.GrpcClient.client.StreamAddDel(context.Background(), &req)
	return endpoint, stream
}
