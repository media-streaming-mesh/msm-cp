package rtsp

import (
	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/sirupsen/logrus"
)

type StubConnection struct {
	conn   pb.MsmControlPlane_SendServer
	addCh  chan *pb.Message
	dataCh chan *base.Response
}

type RTSPConnection struct {
	logger       *logrus.Logger
	author       string
	targetLocal  string
	targetRemote string
	targetAddr   string
	session      map[string]*RTSPSession
}

func newRTSPConnection(log *logrus.Logger) *RTSPConnection {
	return &RTSPConnection{
		logger:  log,
		session: make(map[string]*RTSPSession),
	}
}
