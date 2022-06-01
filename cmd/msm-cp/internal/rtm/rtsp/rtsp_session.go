package rtsp

import (
	"encoding/binary"
	"github.com/sirupsen/logrus"
	"math/rand"
	"strconv"
)

// ServerSession is a server-side RTSP session.
type RTSPSession struct {
	logger *logrus.Logger
	id     string
}

// ID returns the public ID of the session.
func (s *RTSPSession) ID() string {
	return s.id
}

func newSessionID(sessions map[string]*RTSPSession) (string, error) {
	for {
		b := make([]byte, 4)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}

		id := strconv.FormatUint(uint64(binary.LittleEndian.Uint32(b)), 10)

		if _, ok := sessions[id]; !ok {
			return id, nil
		}
	}
}

func newRTSPSession(
	id string,
) *RTSPSession {

	rs := &RTSPSession{
		id: id,
	}

	return rs
}
