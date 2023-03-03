package rtsp

import (
	"github.com/aler9/gortsplib/pkg/base"
	dp "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	"github.com/sirupsen/logrus"
)

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
	response     map[RTSPConnectionState]*base.Response
	responseErr  map[RTSPConnectionState]error
}

type RTSPConnectionState int

const (
	Create   RTSPConnectionState = 0
	Options                      = 1
	Describe                     = 2
	Setup                        = 3
	Play                         = 4
	Teardown                     = 5
)

func newRTSPConnection(log *logrus.Logger) *RTSPConnection {
	return &RTSPConnection{
		logger:      log,
		state:       Create,
		session:     make(map[string]*RTSPSession),
		response:    make(map[RTSPConnectionState]*base.Response),
		responseErr: make(map[RTSPConnectionState]error),
	}
}
