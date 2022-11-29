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
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	"google.golang.org/grpc/peer"
)

// called when a connection is opened.
func (r *RTSP) OnRegistration(server pb.MsmControlPlane_SendServer) {

	// get remote ip addr
	ctx := server.Context()
	p, _ := peer.FromContext(ctx)
	remoteAddr, _, _ := net.SplitHostPort(p.Addr.String())

	sc := &StubConnection{
		conn:    server,
		addCh:   make(chan *pb.Message, 1),
		dataCh:  make(chan *base.Response, 1),
		clients: make(map[string]string),
	}

	//Send node ip address to stub
	dataplaneIP, err := node_mapper.MapNode(remoteAddr)
	r.logger.Debugf("Send msm-proxy ip %v:8050 to %v", dataplaneIP, remoteAddr)
	if err == nil {
		configMsg := &pb.Message{
			Event:  pb.Event_CONFIG,
			Remote: fmt.Sprintf("%s:8050", dataplaneIP),
		}
		sc.conn.Send(configMsg)
	}

	// save stub connection on a sync.Map
	r.stubConn.Store(remoteAddr, sc)
	r.logger.Infof("Connection for client: %s successfully registered", remoteAddr)
}

// called when a connection is opened.
func (r *RTSP) OnConnOpen(server pb.MsmControlPlane_SendServer, msg *pb.Message) {

	// create a new RTSP connection and store it
	sc := newRTSPConnection(r.logger)
	stubAddr := getRemoteIPv4Address(msg.Remote)
	sc.author = msg.Remote

	key := getRTSPConnectionKey(msg.Local, msg.Remote)
	r.rtspConn.Store(key, sc)
	r.logger.Infof("RTSP connection key %v", key)
	r.logger.Infof("RTSP connection opened from client %s", msg.Remote)

	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		r.logger.Errorf("stub connection was not found! %v", stubAddr)
		r.OnExternalClientConnOpen(server, msg)
		return
	}
	srv.(*StubConnection).data = *msg
	srv.(*StubConnection).addCh <- msg
	r.logger.Infof("StubConnection open channel %v", msg)
}

// called when a connection is close.
func (r *RTSP) OnConnClose(server pb.MsmControlPlane_SendServer, msg *pb.Message) {
	//update rtsp state
	rc, _ := r.getClientRTSPConnection(msg)
	if rc.state != Teardown {
		rc.state = Teardown
		//Send DELETE_EP to msm-proxy
		if rc.targetAddr != "" {
			if err := r.SendProxyData(msg); err != nil {
				r.logger.Errorf("Could not send proxy data %v", err)
			}
		}
	}

	// For client RTSP, read from channel to unblock write
	if rc.targetAddr != "" {
		stubAddr := getRemoteIPv4Address(msg.Remote)
		srv, ok := r.stubConn.Load(stubAddr)
		if ok {
			r.logger.Infof("Unblock write channel for %v", msg.Remote)
			<-srv.(*StubConnection).addCh
		} else {
			r.logger.Errorf("stub connection was not found! %v", stubAddr)
			r.OnExternalClientConnClose(server, stubAddr)
		}
	}

	// find RTSP connection and delete it
	key := getRTSPConnectionKey(msg.Local, msg.Remote)
	r.rtspConn.Delete(key)

	r.logger.Infof("RTSP connection closed from client %s", msg.Remote)
}

func (r *RTSP) OnExternalClientConnOpen(server pb.MsmControlPlane_SendServer, msg *pb.Message) {
	// get remote ip addr
	ctx := server.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())
	remoteAddr := getRemoteIPv4Address(msg.Remote)

	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		r.logger.Errorf("gateway stub connection was not found! %v", stubAddr)
		return
	}
	srv.(*StubConnection).clients[remoteAddr] = remoteAddr

	key := getRTSPConnectionKey(msg.Local, msg.Remote)
	rc, ok := r.rtspConn.Load(key)
	if ok {
		messageData := srv.(*StubConnection).data
		rc.(*RTSPConnection).targetLocal = messageData.Local
		rc.(*RTSPConnection).targetRemote = messageData.Remote
	}

	r.logger.Infof("Connection for external client: %s successfully open", remoteAddr)
}

func (r *RTSP) OnExternalClientConnClose(server pb.MsmControlPlane_SendServer, remoteAddr string) {
	// get remote ip addr
	ctx := server.Context()
	p, _ := peer.FromContext(ctx)
	stubAddr, _, _ := net.SplitHostPort(p.Addr.String())

	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		r.logger.Errorf("gateway stub connection was not found! %v", stubAddr)
		return
	}
	delete(srv.(*StubConnection).clients, remoteAddr)
	r.logger.Infof("Connection for external client: %s successfully close", remoteAddr)
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
		req.URL = r.updateURLIpAddress(req.URL)
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

	// store client ports
	clientPorts := getClientPorts(req.Header["Transport"])
	clientEp := getRemoteIPv4Address(s.Remote)
	r.rtpPort.Store(clientEp, clientPorts[0])

	if s_rc.state < Setup {
		r.logger.Debugf("RTSPConnection connection state not SETUP")
		r.logger.Debugf("client header = %v", req.Header)

		// grab client ports
		hdr := req.Header["Transport"][0]
		ports := strings.Split(hdr, "=")[1]
		interleaved := isInterleaved(req.Header["Transport"])

		if interleaved == false {
			// will need to be able to assign other channel values
			req.Header["Transport"] = base.HeaderValue{"RTP/AVP/TCP;unicast;interleaved=0-1"}
			r.logger.Debugf("server header = %v", req.Header)
		}

		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] SETUP RESPONSE %+v", res)

		if interleaved == false {
			// do we need to figure out the SSRC here?
			res.Header["Transport"] = base.HeaderValue{"RTP/AVP;unicast;client_port=" + ports + ";server_port=8050-8051"}
		}

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

// called after receiving a PAUSE request.
func (r *RTSP) OnPause(req *base.Request, s *pb.Message) (*base.Response, error) {
	r.logger.Debugf("[c->s] %+v", req)

	res, err := r.clientToServer(req, s)
	r.logger.Debugf("[s->c] PAUSE RESPONSE %+v", res)

	return res, err
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

	rc, err := r.getClientRTSPConnection(s)
	if err != nil {
		return nil, err
	}

	serverEp := getRemoteIPv4Address(rc.targetRemote)
	data, _ := r.rtspStream.Load(serverEp)
	if data == nil {
		return nil, errors.New("Can't find RTSP stream")
	}

	//update rtsp state
	rc.state = Teardown

	//Send DELETE_EP to msm-proxy for last client
	if err := r.SendProxyData(s); err != nil {
		r.logger.Errorf("Could not send proxy data %v", err)
	}

	//Send TEARDOWN to server if last client
	if len(data.(RTSPStream).clients) == 0 {
		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] TEARDOWN RESPONSE %+v", res)
		return res, err
	}

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

	// 2. Find remote host for endpoint
	host, err := r.getHostFromEndpoint(ep)
	if err != nil {
		r.logger.Errorf("could not find host")
		// res := &base.Response { bad request or something}
		return nil, err
	}

	// 3. Find stub connection for given path
	srv, ok := r.stubConn.Load(host)
	if !ok {
		r.logger.Errorf("could not find stub connection for endpoint")
		return nil, errors.New("not found")
	}

	// 4. Check if remote endpoint open RTSP connection
	if !r.isConnectionOpen(host, s) {
		r.logger.Debugf("Send REQUEST event open RTSP connection for %v", host)
		// Send REQUEST event to server pod
		// _, port, err := net.SplitHostPort(req.URL.Host)
		// if err != nil {
		// 	r.logger.Errorf("could not split host port")
		// 	return nil, err
		// }
		addMsg := &pb.Message{
			Event:  pb.Event_REQUEST,
			Remote: ep,
		}
		srv.(*StubConnection).conn.Send(addMsg)

		// Waiting for server pod response with local/remote ports
		// CP will receive Event_ADD and send value to addCh to unblock channel
		<-srv.(*StubConnection).addCh
	} else {
		r.logger.Debugf("Remote endpoint RTSP connection open")
	}

	// 5. Update target to client pod
	messageData := srv.(*StubConnection).data
	rc, err := r.getClientRTSPConnection(s)
	if err != nil {
		return nil, err
	}
	rc.state = Options
	rc.targetAddr = host
	rc.targetLocal = messageData.Local
	rc.targetRemote = messageData.Remote

	s_rc, err := r.getRemoteRTSPConnection(s)
	if err != nil {
		return nil, err
	}
	r.logger.Debugf("Server state %v", s_rc.state)

	if s_rc.state < Options {
		// 6. Forward OPTIONS command to server pod
		data := bytes.NewBuffer(make([]byte, 0, 4096))
		req.URL = r.updateURLIpAddress(req.URL)
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

		// Update remote RTSP Connection
		r.logger.Debugf("Going to update server state")
		s_rc.state = Options
		s_rc.response[Options] = res
		s_rc.responseErr[Options] = err

		// Log
		r.logger.Debugf("[s->c] OPTIONS RESPONSE %+v", res)
	}

	return s_rc.response[Options], nil
}

func (r *RTSP) clientToServer(req *base.Request, s *pb.Message) (*base.Response, error) {
	key := getRTSPConnectionKey(s.Local, s.Remote)
	sc, ok := r.rtspConn.Load(key)
	if !ok {
		return nil, errors.New("Can't load rtsp connection")
	}

	stubAddr := sc.(*RTSPConnection).targetAddr
	srv, ok := r.stubConn.Load(stubAddr)
	if !ok {
		return nil, errors.New("can't load stub connection")
	}

	data := bytes.NewBuffer(make([]byte, 0, 4096))
	req.Write(data)

	r.logger.Debugf("Sending data from client to server %v", stubAddr)
	srv.(*StubConnection).conn.Send(&pb.Message{
		Event:  pb.Event_DATA,
		Local:  sc.(*RTSPConnection).targetLocal,
		Remote: sc.(*RTSPConnection).targetRemote,
		Data:   fmt.Sprintf("%s", data),
	})

	r.logger.Debugf("Sent data to server")

	res := <-srv.(*StubConnection).dataCh

	return res, nil
}

func (r *RTSP) getHostFromEndpoint(ep string) (string, error) {
	host, _, err := net.SplitHostPort(ep)
	if err != nil {
		return "", errors.New("could not parse host:port")
	}

	r.logger.Debugf("remote IP to connect: %s", host)

	return host, nil
}

func (r *RTSP) getEndpointFromPath(p *base.URL) (string, error) {
	urls := r.urlHandler.GetInternalURLs(p.String())

	r.logger.Debugf("endpoints to connect: %v", urls)

	ep, err := base.ParseURL(urls[0])
	if err != nil {
		return "", errors.New("could not parse endpoint")
	}
	// host, _, err := net.SplitHostPort(ep.Host)
	// if err != nil {
	// 	return "", errors.New("could not parse host:port")
	// }
	r.logger.Debugf("endpoint to connect: %s", ep)

	return ep.Host, nil
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

func (r *RTSP) isConnectionOpen(ep string, s *pb.Message) bool {
	r.logger.Debugf("Check RTSP connection for endpoint %s", ep)
	check := false

	_, err := r.getRemoteRTSPConnection(s)
	if err != nil {
		return false
	}

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

func (r *RTSP) clientCount(clientEp string) int {
	var count = 0
	r.rtspStream.Range(func(serverEp, rtspStream interface{}) bool {
		clients := rtspStream.(RTSPStream).clients
		for _, c := range clients {
			if c == clientEp {
				count = len(clients)
			}
		}
		return true
	})
	return count
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

func (r *RTSP) getStubAddress(ep string) string {
	var stubAddress = ""
	r.stubConn.Range(func(key, srv interface{}) bool {
		value := srv.(*StubConnection).clients[ep]
		if value == ep {
			stubAddress = key.(string)
		}
		return true
	})
	return stubAddress
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

func isInterleaved(value base.HeaderValue) bool {
	var interleaved = false
	for _, transportValue := range value {
		transportValues := strings.Split(transportValue, ";")
		for _, transportL2Value := range transportValues {
			if strings.Contains(transportL2Value, "interleaved") {
				interleaved = true
				break
			}
		}
	}
	return interleaved
}

func (r *RTSP) updateURLIpAddress(url *base.URL) *base.URL {
	ep, err := r.getEndpointFromPath(url)
	if err != nil {
		r.logger.Errorf("could not find endpoint")
		return url
	}
	// url.Host = fmt.Sprintf("%s:554", ep)
	url.Host = ep
	r.logger.Debugf("Update url to %v", url)
	return url
}
