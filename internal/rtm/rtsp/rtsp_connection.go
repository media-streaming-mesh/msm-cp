package rtsp

import (
	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	dp "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/sirupsen/logrus"
)

type StubConnection struct {
	conn   pb.MsmControlPlane_SendServer
	data   pb.Message
	addCh  chan *pb.Message
	dataCh chan *base.Response
}

type RTPProxyConnection struct {
	conn       dp.MsmDataPlaneClient
	streamCh   chan *dp.StreamData
	responseCh chan *base.Response
}

type RTSPConnection struct {
	logger       *logrus.Logger
	state        RTSPConnectionState
	author       string
	targetLocal  string
	targetRemote string
	targetAddr   string
	session      map[string]*RTSPSession
	lastResponse *base.Response
	lastError    error
}

type RTSPConnectionState int

const (
	Create   RTSPConnectionState = 0
	Describe                     = 1
	Options                      = 2
	Setup                        = 3
	Play                         = 4
)

func newRTSPConnection(log *logrus.Logger) *RTSPConnection {
	return &RTSPConnection{
		logger:  log,
		state:   Create,
		session: make(map[string]*RTSPSession),
	}
}
