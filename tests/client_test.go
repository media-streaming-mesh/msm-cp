package transport

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"

	pb_dp "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/media-streaming-mesh/msm-cp/internal/transport"
)

func TestCreateStream(t *testing.T) {
	logger := logrus.New()

	grpcClient, err := transport.SetupClient("172.18.0.2")
	require.NoError(t, err)
	if err != nil {
		logger.Debugf("Failed to connect to server, error %s\n", err)
	}
	dpGrpcClient := transport.Client{
		logger,
		grpcClient,
	}

	t.Log("FIXME")
	stream, result := dpGrpcClient.CreateStream(1, pb_dp.Encap_RTP_UDP, "192.168.82.20", 8050)
	logger.Debugf("Create stream-mapper %v %v", stream, result)
}
