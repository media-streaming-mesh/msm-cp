package transport

import (
	"context"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/sirupsen/logrus"
	"strconv"
)

// Client holds the client specific data structures
type Client struct {
	Log        *logrus.Logger
	GrpcClient *grpcClient
}

func SetupClient() (*grpcClient, error) {
	grpcClient, err := newGRPCClient()
	grpcClient.start()
	return grpcClient, err
}

func (c *Client) CreateStream(ip string, port uint32) error {
	//Prepare CREATE data
	encap, _ := strconv.ParseUint(pb.Encap_RTP_UDP.String(), 10, 32)

	endpoint := pb.Endpoint{
		Ip:    ip,
		Port:  port,
		Encap: uint32(encap),
	}
	req := pb.StreamData{
		Id:        1, //TODO: autogenerate stream ID
		Operation: pb.StreamOperation_CREATE,
		Protocol:  pb.ProxyProtocol_RTP,
		Endpoint:  &endpoint,
	}

	//Send data to RTPProxy
	stream, err := c.GrpcClient.client.StreamAddDel(context.Background(), &req)
	c.Log.Debug("Send stream %v", stream)

	if err != nil {
		c.Log.Debugf("Send stream error")
	}
	c.Log.Debugf("Finished send stream")
	return err
}

func (c *Client) CreateEndpoint(ip string, port uint32) error {
	//Prepare AddEndpoint data
	encap, _ := strconv.ParseUint(pb.Encap_RTP_UDP.String(), 10, 32)

	endpoint := pb.Endpoint{
		Ip:    ip,
		Port:  port,
		Encap: uint32(encap),
	}
	req := pb.StreamData{
		Id:        1, //TODO: autogenerate stream ID
		Operation: pb.StreamOperation_ADD_EP,
		Endpoint:  &endpoint,
	}

	//Send data to RTPProxy
	stream, err := c.GrpcClient.client.StreamAddDel(context.Background(), &req)
	c.Log.Debug("Send stream %v", stream)
	return err
}
