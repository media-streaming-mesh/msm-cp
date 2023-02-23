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
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"log"
	"net"
	"strconv"
	"strings"

	"github.com/aler9/gortsplib/pkg/base"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
)

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
	originalURL := req.URL.String()

	if s_rc.state < Describe {
		r.logger.Debugf("RTSPConnection connection state not DESCRIBE")

		req.URL = r.updateURLIpAddress(req.URL)
		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] DESCRIBE RESPONSE %+v", res)

		s_rc.state = Describe
		s_rc.response[Describe] = res
		s_rc.responseErr[Describe] = err
	}

	s_rc.response[Describe].Header["Content-Base"] = base.HeaderValue{originalURL}
	r.logger.Debugf("[s->c] updated DESCRIBE RESPONSE %+v", s_rc.response[Describe])

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

	// grab client ports
	hdr := req.Header["Transport"][0]
	ports := strings.Split(hdr, "=")[1]
	interleaved := isInterleaved(req.Header["Transport"])

	if s_rc.state < Setup {
		r.logger.Debugf("RTSPConnection connection state not SETUP")
		r.logger.Debugf("client header = %v", req.Header)

		if interleaved == false {
			// will need to be able to assign other channel values
			// always interleaved towards the server (for now)
			req.Header["Transport"] = base.HeaderValue{"RTP/AVP/TCP;unicast;interleaved=0-1"}
			r.logger.Debugf("server header = %v", req.Header)
		}

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

	if interleaved {
		s_rc.response[Setup].Header["Transport"] = base.HeaderValue{"RTP/AVP/TCP;unicast;interleaved=0-1"}
	} else {
		// do we need to figure out the SSRC here?
		s_rc.response[Setup].Header["Transport"] = base.HeaderValue{"RTP/AVP;unicast;client_port=" + ports + ";server_port=8050-8051"}
	}

	r.logger.Debugf("modified setup response is %v", s_rc.response[Setup])

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
		Header:     make(base.Header),
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

	//Delete client from clientMap
	connectionKey := getRTSPConnectionKey(s.Local, s.Remote)
	delete(r.clientMap, connectionKey)

	rc, err := r.getClientRTSPConnection(s)
	if err != nil {
		return nil, err
	}

	//update rtsp state
	rc.state = Teardown

	//Send TEARDOWN to server if last client
	serverEp := getRemoteIPv4Address(rc.targetRemote)
	if r.getClientCount(serverEp) == 0 {
		res, err := r.clientToServer(req, s)
		r.logger.Debugf("[s->c] TEARDOWN RESPONSE %+v", res)
		return res, err
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     make(base.Header),
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
	stubConn, ok := model.StubMap.Load(host)
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
		stubConn.(*model.StubConnection).Conn.Send(addMsg)

		// Waiting for server pod response with local/remote ports
		// CP will receive Event_ADD and send value to addCh to unblock channel
		<-stubConn.(*model.StubConnection).AddCh
		stubConn.(*model.StubConnection).SendToAddCh = false
	} else {
		r.logger.Debugf("Remote endpoint RTSP connection open")
	}

	// 5. Update target to client pod
	messageData := stubConn.(*model.StubConnection).Data
	rc, err := r.getClientRTSPConnection(s)
	if err != nil {
		return nil, err
	}
	rc.state = Options
	rc.targetAddr = host
	rc.targetLocal = messageData.Local
	rc.targetRemote = messageData.Remote

	s_key := getRTSPConnectionKey(rc.targetLocal, rc.targetRemote)
	data, _ := r.rtspConn.Load(s_key)
	if data == nil {
		return nil, errors.New("Can't find server RTSP connection")
	}
	s_rc := data.(*RTSPConnection)

	//Update client map
	connectionKey := getRTSPConnectionKey(s.Local, s.Remote)
	client := r.clientMap[connectionKey]
	r.clientMap[connectionKey] = Client{
		getRemoteIPv4Address(s.Remote),
		client.clientPorts,
		host,
	}

	if s_rc.state < Options {
		// 6. Forward OPTIONS command to server pod
		data := bytes.NewBuffer(make([]byte, 0, 4096))
		req.Write(data)

		optionsMsg := &pb.Message{
			Event:  pb.Event_DATA,
			Local:  messageData.Local,
			Remote: messageData.Remote,
			Data:   fmt.Sprintf("%s", data),
		}

		stubConn.(*model.StubConnection).Conn.Send(optionsMsg)
		r.logger.Debugf("waiting on options response")
		res := <-stubConn.(*model.StubConnection).DataCh

		// Update remote RTSP Connection
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
	stubConn, ok := model.StubMap.Load(stubAddr)
	if !ok {
		return nil, errors.New("can't load stub connection")
	}

	data := bytes.NewBuffer(make([]byte, 0, 4096))
	req.Write(data)

	stubConn.(*model.StubConnection).Conn.Send(&pb.Message{
		Event:  pb.Event_DATA,
		Local:  sc.(*RTSPConnection).targetLocal,
		Remote: sc.(*RTSPConnection).targetRemote,
		Data:   fmt.Sprintf("%s", data),
	})

	res := <-stubConn.(*model.StubConnection).DataCh

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

	if len(urls) == 0 {
		return "", errors.New("Can't get endpoint from path")
	}

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
		return nil, errors.New("Can't find remote client RTSP connection")
	}

	// Server RTSPConnection
	s_key := getRTSPConnectionKey(rc.targetLocal, rc.targetRemote)
	s_rc, s_ok := r.rtspConn.Load(s_key)
	if !s_ok {
		return nil, errors.New("Can't find remote server RTSP connection")
	}
	return s_rc.(*RTSPConnection), nil
}

func (r *RTSP) getClientCount(serverEp string) int {
	count := 0
	for _, v := range r.clientMap {
		if v.serverIp == serverEp {
			count++
		}
	}
	return count
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
