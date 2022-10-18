package transport

import (
	"github.com/media-streaming-mesh/msm-cp/internal/transport"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"testing"
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

	stream, result := dpGrpcClient.CreateStream("192.168.82.20", 8050)
	logger.Debugf("Create stream %v %v", stream, result)
}
