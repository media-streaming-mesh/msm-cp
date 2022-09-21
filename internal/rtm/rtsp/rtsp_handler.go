/*
* Copyright (c) 2022-2022 Cisco and/or its affiliates.
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at:
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package rtsp

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"google.golang.org/grpc/peer"
	"log"
	"net"
	"strconv"
	"strings"
)

// called when a connection is opened.
func (r *RTSP) OnRegistration(server pb.MsmControlPlane_SendServer) {

	// get remote ip addr
	ctx := server.Context()
	p, _ := peer.FromContext(ctx)
	remoteAddr, _, _ := net.SplitHostPort(p.Addr.String())

	sc := &StubConnection{
		conn:   server,
		addCh:  make(chan *pb.Message, 1),
		dataCh: make(chan *base.Response, 1),
	}

	// save stub connection on a sync.Map
	r.stubConn.Store(remoteAddr, sc)
	r.logger.Infof("Connection for client: %s successfully registered", remoteAddr)
}

// called when a connection is opened.
func (r *RTSP) OnConnOpen(msg *pb.Message) {

	// create a new RTSP connection and store it
	sc := newRTSPConnection(r.logger)
	stubAddr := getRemoteIPv4Address(msg.Remote)
	sc.author = msg.Remote

	key := getRTSPConnectionKey(msg.Local, msg.Remote)
	r.rtspConn.Store(key, sc)

	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		r.logger.Errorf("stub connection was not found!")
		return
	}
	srv.(*StubConnection).data = *msg
	srv.(*StubConnection).addCh <- msg
	r.logger.Infof("RTSP connection key %v", key)
	r.logger.Infof("RTSP connection opened from client %s", msg.Remote)
}

// called when a connection is close.
func (r *RTSP) OnConnClose(msg *pb.Message) {

	// find RTSP connection and delete it
	key := getRTSPConnectionKey(msg.Local, msg.Remote)
	r.rtspConn.Delete(key)

	// read from channel to unblock write
	stubAddr := getRemoteIPv4Address(msg.Remote)
	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		r.logger.Errorf("stub connection was not found!")
		return
	}
	<-srv.(*StubConnection).addCh
	r.logger.Infof("RTSP connection closed from client %s", msg.Remote)
}

// called when a session is opened.
func (r *RTSP) OnSessionOpen() {
	// save session
}

// called when a session is closed.
func (r *RTSP) OnSessionClose() {
	log.Printf("session closed")

}

// called after receiving an OPTIONS request.
func (r *RTSP) OnOptions(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	// call k8sAPIHelper to connect to server pod
	res, err := r.connectToRemote(req, s)
	if err != nil {
		// handle error
		// res := &base.Response { bad request or something}
		return nil, err
	}
	return res, nil

}

// called after receiving a DESCRIBE request.
func (r *RTSP) OnDescribe(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	rc, _ := r.getClientRTSPConnection(s)
	rc.state = Describe

	s_rc, error := r.getRemoteRTSPConnection(s)
	if error != nil {
		return nil, error
	}
	if s_rc.state < Describe {
		r.logger.Debugf("RTSPConnection connection state not DESCRIBE")
		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] DESCRIBE RESPONSE %+v", res)

		s_rc.state = Describe
		s_rc.response[Describe] = res
		s_rc.responseErr[Describe] = err
	}

	return s_rc.response[Describe], s_rc.responseErr[Describe]
}

// called after receiving an ANNOUNCE request.
func (r *RTSP) OnAnnounce(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)
	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[s->c] ANNOUNCE RESPONSE %+v", res)

	return res, err
}

// called after receiving a SETUP request.
func (r *RTSP) OnSetup(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	rc, _ := r.getClientRTSPConnection(s)
	rc.state = Setup

	s_rc, error := r.getRemoteRTSPConnection(s)
	if error != nil {
		return nil, error
	}

	if s_rc.state < Setup {
		r.logger.Debugf("RTSPConnection connection state not SETUP")

		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] SETUP RESPONSE %+v", res)

		//If stream contains both video and audio, wait for both stream finish setup
		//  before setup RTPProxy
		describeResponse := s_rc.response[Describe]
		if strings.Contains(describeResponse.String(), "trackID=1") && s_rc.response[Setup] == nil {
			r.logger.Debugf("RTSPConnection connection has both audio and video")
		} else {
			s_rc.state = Setup
		}
		s_rc.response[Setup] = res
		s_rc.responseErr[Setup] = err
	}

	return s_rc.response[Setup], s_rc.responseErr[Setup]
}

// called after receiving a PLAY request.
func (r *RTSP) OnPlay(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	rc, _ := r.getClientRTSPConnection(s)
	rc.state = Play

	s_rc, error := r.getRemoteRTSPConnection(s)
	if error != nil {
		return nil, error
	}

	if s_rc.state < Play {
		r.logger.Debugf("RTSPConnection connection state not PLAY")
		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] PLAY RESPONSE %+v", res)

		s_rc.state = Play
		s_rc.response[Play] = res
		s_rc.responseErr[Play] = err
	}

	return s_rc.response[Play], s_rc.responseErr[Play]
}

// called after receiving a RECORD request.
func (r *RTSP) OnRecord(req *base.Request) (*base.Response, error) {
	log.Printf("record request")

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

// called after receiving a GET_PARAMETER request.
func (r *RTSP) OnGetParameter(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[s->c] GET_PARAMETER RESPONSE %+v", res)

	return res, err
}

// called after receiving a TEARDOWN request.
func (r *RTSP) OnTeardown(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	//res, err := r.clientToServer(req, s)
	//r.logger.Debugf("[s->c] TEARDOWN RESPONSE %+v", res)

	//return res, err
	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func (r *RTSP) connectToRemote(req *base.Request, s *pb.Message) (*base.Response, error) {

	// 1. Find the remote endpoint to connect
	ep, err := r.getEndpointFromPath(req.URL)
	if err != nil {
		r.logger.Errorf("could not find endpoint")
		// res := &base.Response { bad request or something}
		return nil, err
	}

	// 2. Find stub connection for given path
	srv, ok := r.stubConn.Load(ep)
	if !ok {
		r.logger.Errorf("could not find stub connection for endpoint")
		return nil, errors.New("not found")
	}

	// 3. Check if remote endpoint open RTSP connection
	if !r.isConnectionOpen(ep) {
		r.logger.Debugf("Send ADD event open RTSP connection for %v", ep)
		// Send ADD event to server pod
		_, port, _ := net.SplitHostPort(req.URL.Host)
		addMsg := &pb.Message{
			Event:  pb.Event_ADD,
			Remote: fmt.Sprintf("%s:%s", ep, port),
		}
		srv.(*StubConnection).conn.Send(addMsg)

		// Waiting for server pod response with local/remote ports
		// CP will receive Event_ADD and send value to addCh to unblock channel
		<-srv.(*StubConnection).addCh
	} else {
		r.logger.Debugf("Remote endpoint RTSP connection open")
	}

	// 4. Update target to client pod
	messageData := srv.(*StubConnection).data
	rc, _ := r.getClientRTSPConnection(s)
	rc.state = Options
	rc.targetAddr = ep
	rc.targetLocal = messageData.Local
	rc.targetRemote = messageData.Remote

	s_rc, _ := r.getRemoteRTSPConnection(s)
	if s_rc.state < Options {
		// 5. Forward OPTIONS command to sever pod
		data := bytes.NewBuffer(make([]byte, 0, 4096))
		req.Write(data)

		optionsMsg := &pb.Message{
			Event:  pb.Event_DATA,
			Local:  messageData.Local,
			Remote: messageData.Remote,
			Data:   fmt.Sprintf("%s", data),
		}

		srv.(*StubConnection).conn.Send(optionsMsg)
		r.logger.Debugf("waiting on options response")
		res := <-srv.(*StubConnection).dataCh

		//Update remote RTSTConnection
		s_rc.state = Options
		s_rc.response[Options] = res
		s_rc.responseErr[Options] = err

		//Log
		r.logger.Debugf("[s->c] OPTIONS RESPONSE %+v", res)
	}

	return s_rc.response[Options], nil
}

func (r *RTSP) clientToServer(req *base.Request, s *pb.Message) (*base.Response, error) {
	key := getRTSPConnectionKey(s.Local, s.Remote)
	sc, ok := r.rtspConn.Load(key)
	if !ok {
		return nil, errors.New("shit3")
	}

	stubAddr := sc.(*RTSPConnection).targetAddr
	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		return nil, errors.New("shit5")
	}

	data := bytes.NewBuffer(make([]byte, 0, 4096))
	req.Write(data)

	r.logger.Debugf("Sending data from client %v to server", stubAddr)
	srv.(*StubConnection).conn.Send(&pb.Message{
		Event:  pb.Event_DATA,
		Local:  sc.(*RTSPConnection).targetLocal,
		Remote: sc.(*RTSPConnection).targetRemote,
		Data:   fmt.Sprintf("%s", data),
	})

	res := <-srv.(*StubConnection).dataCh
	return res, nil
}

func (r *RTSP) getEndpointFromPath(p *base.URL) (string, error) {
	urls := r.urlHandler.GetInternalURLs(p.String())

	r.logger.Debugf("endpoints to connect: %v", urls)

	ep, err := base.ParseURL(urls[0])
	if err != nil {
		return "", errors.New("could not parse endpoint")
	}
	host, _, err := net.SplitHostPort(ep.Host)
	if err != nil {
		return "", errors.New("could not parse host:port")
	}
	r.logger.Debugf("endpoint to connect: %s", ep)

	return host, nil
}

func getRemoteIPv4Address(url string) string {
	res := strings.ReplaceAll(url, "[", "")
	res = strings.ReplaceAll(res, "]", "")
	n := strings.LastIndex(res, ":")

	return fmt.Sprintf("%s", net.ParseIP(res[:n]))
}

func getRTSPConnectionKey(s1, s2 string) string {
	return fmt.Sprintf("%s%s", s1, s2)
}

func (r *RTSP) isConnectionOpen(ep string) bool {
	r.logger.Debugf("Check RTSP connection for endpoint %s", ep)
	check := false
	r.rtspConn.Range(func(key, value interface{}) bool {
		r.logger.Debugf("RTSP connection targetAddress %s", value.(*RTSPConnection).targetAddr)
		r.logger.Debugf("RTSP connection localAddress %s", value.(*RTSPConnection).targetLocal)
		r.logger.Debugf("RTSP connection remoteAddress %s", value.(*RTSPConnection).targetRemote)
		if value.(*RTSPConnection).targetAddr == ep {
			check = true
		}
		return true
	})
	return check
}

func (r *RTSP) getClientRTSPConnection(s *pb.Message) (*RTSPConnection, error) {
	// Client RTSPConnection
	key := getRTSPConnectionKey(s.Local, s.Remote)
	rc, ok := r.rtspConn.Load(key)
	if !ok {
		return nil, errors.New("Can't find client RTSP connection")
	}
	return rc.(*RTSPConnection), nil
}

func (r *RTSP) getRemoteRTSPConnection(s *pb.Message) (*RTSPConnection, error) {
	// Client RTSPConnection
	rc, err := r.getClientRTSPConnection(s)
	if err != nil {
		return nil, errors.New("Can't find client RTSP connection")
	}

	// Server RTSPConnection
	s_key := getRTSPConnectionKey(rc.targetLocal, rc.targetRemote)
	s_rc, s_ok := r.rtspConn.Load(s_key)
	if !s_ok {
		return nil, errors.New("Can't find server RTSP connection")
	}
	return s_rc.(*RTSPConnection), nil
}

func getClientPorts(value base.HeaderValue) []uint32 {
	var ports []uint32
	for _, transportValue := range value {
		transportValues := strings.Split(transportValue, ";")
		for _, transportL2Value := range transportValues {
			if strings.Contains(transportL2Value, "interleaved") {
				//TODO: return 8051 when transport return RTCP
				ports = append(ports, uint32(8050))
			}
			if strings.Contains(transportL2Value, "client_port") {
				transportL2Values := strings.Split(transportL2Value, "=")
				for _, transportL3Value := range transportL2Values {
					transportL3Values := strings.Split(transportL3Value, "-")
					for _, transportL4Value := range transportL3Values {
						if port, err := strconv.ParseUint(transportL4Value, 10, 32); err == nil {
							ports = append(ports, uint32(port))
						}
					}
				}
			}
		}
	}
	return ports
}

func getServerPorts(value base.HeaderValue) []uint32 {
	var ports []uint32
	for _, transportValue := range value {
		transportValues := strings.Split(transportValue, ";")
		for _, transportL2Value := range transportValues {
			if strings.Contains(transportL2Value, "interleaved") {
				//TODO: return 8051 when transport return RTCP
				ports = append(ports, uint32(8050))
			}
			if strings.Contains(transportL2Value, "server_port") {
				transportL2Values := strings.Split(transportL2Value, "=")
				for _, transportL3Value := range transportL2Values {
					transportL3Values := strings.Split(transportL3Value, "-")
					for _, transportL4Value := range transportL3Values {
						if port, err := strconv.ParseUint(transportL4Value, 10, 32); err == nil {
							ports = append(ports, uint32(port))
						}
					}
				}
			}
		}
	}
	return ports
}
