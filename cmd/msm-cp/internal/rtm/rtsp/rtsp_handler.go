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

	srv.(*StubConnection).addCh <- msg
	r.logger.Infof("RTSP connection opened from client %s", msg.Remote)
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

	var methods []string
	methods = append(methods, string(base.Describe))
	methods = append(methods, string(base.Announce))
	methods = append(methods, string(base.Setup))
	methods = append(methods, string(base.Play))
	methods = append(methods, string(base.Record))
	methods = append(methods, string(base.Pause))
	methods = append(methods, string(base.Teardown))
	methods = append(methods, string(base.SetParameter))

	//
	res, err := r.connectToRemote(req, s)
	if err != nil {
		// handle error
		// res := &base.Response { bad request or something}
		return nil, err
	}

	// find intersection of options supported
	// wait for channel from other goroutine?
	r.logger.Debugf("[s->c] OPTIONS RESPONSE %+v", res)
	return res, nil

}

// called after receiving a DESCRIBE request.
func (r *RTSP) OnDescribe(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)
	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[s->c] DESCRIBE RESPONSE %+v", res)

	return res, err
}

// called after receiving an ANNOUNCE request.
func (r *RTSP) OnAnnounce(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)
	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[c->s] ANNOUNCE RESPONSE %+v", res)

	return res, err
}

// called after receiving a SETUP request.
func (r *RTSP) OnSetup(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)
	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[s->c] SETUP RESPONSE %+v", res)

	return res, err
}

// called after receiving a PLAY request.
func (r *RTSP) OnPlay(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)
	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[s->c] PLAY RESPONSE %+v", res)

	return res, err
}

// called after receiving a RECORD request.
func (r *RTSP) OnRecord(req *base.Request) (*base.Response, error) {
	log.Printf("record request")

	return &base.Response{
		StatusCode: base.StatusOK,
	}, nil
}

func (r *RTSP) connectToRemote(req *base.Request, s *pb.Message) (*base.Response, error) {

	// 1. Find the backend to connect to and save it to the rtsp connection
	path, port, _ := net.SplitHostPort(req.URL.Host)
	ep, err := r.getEndpointFromPath(path)
	if err != nil {
		r.logger.Errorf("could not find endpoint")
		// res := &base.Response { bad request or something}
		return nil, err
	}

	// 2. Find stub connection for given path
	srv, ok := r.stubConn.Load(ep)
	if !ok {
		r.logger.Errorf("could not find stub connection for endpoint")
		return nil, errors.New("shit1")
	}

	addMsg := &pb.Message{
		Event:  pb.Event_ADD,
		Remote: fmt.Sprintf("%s:%s", ep, port),
	}
	srv.(*StubConnection).conn.Send(addMsg)

	addCh := <-srv.(*StubConnection).addCh

	data := bytes.NewBuffer(make([]byte, 0, 4096))
	req.Write(data)

	optionsMsg := &pb.Message{
		Event:  pb.Event_DATA,
		Local:  addCh.Local,
		Remote: addCh.Remote,
		Data:   fmt.Sprintf("%s", data),
	}

	// update target to given pod
	key := getRTSPConnectionKey(s.Local, s.Remote)
	rc, ok := r.rtspConn.Load(key)
	rc.(*RTSPConnection).targetAddr = ep
	rc.(*RTSPConnection).targetLocal = addCh.Local
	rc.(*RTSPConnection).targetRemote = addCh.Remote

	srv.(*StubConnection).conn.Send(optionsMsg)

	r.logger.Debugf("waiting on options response")
	res := <-srv.(*StubConnection).dataCh
	return res, nil
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

	srv.(*StubConnection).conn.Send(&pb.Message{
		Event:  pb.Event_DATA,
		Local:  sc.(*RTSPConnection).targetLocal,
		Remote: sc.(*RTSPConnection).targetRemote,
		Data:   fmt.Sprintf("%s", data),
	})

	res := <-srv.(*StubConnection).dataCh
	return res, nil
}

func (r *RTSP) getEndpointFromPath(p string) (string, error) {
	r.logger.Debugf("K8s API Client response: %s", p)

	ep := r.remote
	return ep, nil
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
