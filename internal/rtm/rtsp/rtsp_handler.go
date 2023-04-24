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
	"errors"
	"fmt"
	"github.com/aler9/gortsplib/pkg/base"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"net"
	"strconv"
	"strings"
)

// called after receiving an OPTIONS request.
func (r *RTSP) OnOptions(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	// call k8sAPIHelper to connect to server pod
	host, err := r.connectToRemote(req)
	if err != nil {
		r.logError("unable to connect to remote")
		return nil, err
	}
	stubChannel, ok := r.stubChannels[host]
	if !ok {
		return nil, errors.New("can't load stub channel")
	}
	serverConnectionKey := stubChannel.Key

	// Update target to client pod
	rc, err := r.getClientRTSPConnection(connectionKey)
	if err != nil {
		return nil, err
	}
	rc.state = Options
	rc.targetAddr = host
	rc.targetLocal = serverConnectionKey.Local
	rc.targetRemote = serverConnectionKey.Remote

	s_rc, err := r.getRemoteRTSPConnection(connectionKey)
	if err != nil {
		return nil, err
	}

	//Update client map
	client := r.clientMap[connectionKey.Key]
	r.clientMap[connectionKey.Key] = Client{
		getRemoteIPv4Address(connectionKey.Remote),
		client.clientPorts,
		host,
	}

	if s_rc.state < Options {
		// Forward OPTIONS command to server pod
		stubChannel.Request <- model.StubChannelRequest{
			model.Data,
			serverConnectionKey.Local,
			serverConnectionKey.Remote,
			req,
		}
		r.log("waiting on options response")
		res := <-stubChannel.Response

		// Update remote RTSP Connection
		s_rc.state = Options
		s_rc.response[Options] = res.Response
		s_rc.responseErr[Options] = err

		// Log option response
		r.log("[s->c] OPTIONS RESPONSE %+v", res)
	}

	return s_rc.response[Options], nil
}

// called after receiving a DESCRIBE request.
func (r *RTSP) OnDescribe(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	rc, _ := r.getClientRTSPConnection(connectionKey)
	rc.state = Describe

	s_rc, error := r.getRemoteRTSPConnection(connectionKey)
	if error != nil {
		return nil, error
	}
	originalURL := req.URL.String()

	if s_rc.state < Describe {
		r.log("RTSPConnection connection state not DESCRIBE")

		req.URL = r.updateURLIpAddress(req.URL)
		res, err := r.clientToServer(req, connectionKey)
		r.log("[s->c] DESCRIBE RESPONSE %+v", res)

		s_rc.state = Describe
		s_rc.response[Describe] = res
		s_rc.responseErr[Describe] = err
	}

	s_rc.response[Describe].Header["Content-Base"] = base.HeaderValue{originalURL}
	r.log("[s->c] updated DESCRIBE RESPONSE %+v", s_rc.response[Describe])

	return s_rc.response[Describe], s_rc.responseErr[Describe]
}

// called after receiving an ANNOUNCE request.
func (r *RTSP) OnAnnounce(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)
	res, err := r.clientToServer(req, connectionKey)
	r.log("[s->c] ANNOUNCE RESPONSE %+v", res)

	return res, err
}

// called after receiving a SETUP request.
func (r *RTSP) OnSetup(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	rc, _ := r.getClientRTSPConnection(connectionKey)
	rc.state = Setup

	s_rc, error := r.getRemoteRTSPConnection(connectionKey)
	if error != nil {
		return nil, error
	}

	// grab client ports
	hdr := req.Header["Transport"][0]
	ports := strings.Split(hdr, "=")[1]
	interleaved := isInterleaved(req.Header["Transport"])

	if s_rc.state < Setup {
		r.log("RTSPConnection connection state not SETUP")
		r.log("client header = %v", req.Header)

		if interleaved == false {
			// will need to be able to assign other channel values
			// always interleaved towards the server (for now)
			req.Header["Transport"] = base.HeaderValue{"RTP/AVP/TCP;unicast;interleaved=0-1"}
			r.log("server header = %v", req.Header)
		}

		res, err := r.clientToServer(req, connectionKey)
		r.log("[s->c] SETUP RESPONSE %+v", res)

		// If stream contains both video and audio, wait for both stream finish setup
		//  before setup RTPProxy
		describeResponse := s_rc.response[Describe]
		if strings.Contains(describeResponse.String(), "trackID=1") && s_rc.response[Setup] == nil {
			r.log("RTSPConnection connection has both audio and video")
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

	r.log("modified setup response is %v", s_rc.response[Setup])

	return s_rc.response[Setup], s_rc.responseErr[Setup]
}

// called after receiving a PLAY request.
func (r *RTSP) OnPlay(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	rc, _ := r.getClientRTSPConnection(connectionKey)
	rc.state = Play

	s_rc, error := r.getRemoteRTSPConnection(connectionKey)
	if error != nil {
		return nil, error
	}

	if s_rc.state < Play {
		r.log("RTSPConnection connection state not PLAY")
		res, err := r.clientToServer(req, connectionKey)
		r.log("[s->c] PLAY RESPONSE %+v", res)

		s_rc.state = Play
		s_rc.response[Play] = res
		s_rc.responseErr[Play] = err
	}

	return s_rc.response[Play], s_rc.responseErr[Play]
}

// called after receiving a PAUSE request.
func (r *RTSP) OnPause(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	res, err := r.clientToServer(req, connectionKey)
	r.log("[s->c] PAUSE RESPONSE %+v", res)

	return res, err
}

// called after receiving a RECORD request.
func (r *RTSP) OnRecord(req *base.Request) (*base.Response, error) {
	r.log("record request")
	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     make(base.Header),
	}, nil
}

// called after receiving a GET_PARAMETER request.
func (r *RTSP) OnGetParameter(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	res, err := r.clientToServer(req, connectionKey)
	r.log("[s->c] GET_PARAMETER RESPONSE %+v", res)

	return res, err
}

// called after receiving a TEARDOWN request.
func (r *RTSP) OnTeardown(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	r.log("[c->s] %+v", req)

	//Delete client from clientMap
	delete(r.clientMap, connectionKey.Key)

	rc, err := r.getClientRTSPConnection(connectionKey)
	if err != nil {
		return nil, err
	}

	// update rtsp state
	rc.state = Teardown

	// Send TEARDOWN to server if last client
	serverEp := getRemoteIPv4Address(rc.targetRemote)
	if r.getClientCount(serverEp) == 0 {
		res, err := r.clientToServer(req, connectionKey)
		r.log("[s->c] TEARDOWN RESPONSE %+v", res)
		return res, err
	}

	return &base.Response{
		StatusCode: base.StatusOK,
		Header:     make(base.Header),
	}, nil
}

func (r *RTSP) connectToRemote(req *base.Request) (string, error) {

	// Find the remote endpoint to connect
	ep, err := r.getEndpointFromPath(req.URL)
	if err != nil {
		r.logError("could not find endpoint to connect to")
		return "", err
	}

	// Find remote host for endpoint
	host, err := r.getHostFromEndpoint(ep)
	if err != nil {
		r.logError("could not find host")
		return "", err
	}

	// 4. Check if remote endpoint open RTSP connection
	if !r.isConnectionOpen(host) {
		stubChannel, ok := r.stubChannels[host]
		if ok {
			r.log("Send REQUEST event open RTSP connection for %v", host)
			stubChannel.Request <- model.StubChannelRequest{
				model.Add,
				"",
				ep,
				nil,
			}

			// Waiting for server pod response with local/remote ports
			// CP will receive Event_ADD and send value to addCh to unblock channel
			<-stubChannel.Response
			r.log("Successful connect to remote")
		} else {
			return "", errors.New("Can't load stub channel")
		}
	} else {
		r.log("Remote endpoint RTSP connection open")
	}
	return host, nil
}

func (r *RTSP) clientToServer(req *base.Request, connectionKey model.ConnectionKey) (*base.Response, error) {
	sc, ok := r.rtspConn.Load(connectionKey.Key)
	if !ok {
		return nil, errors.New("Can't load rtsp connection")
	}

	stubAddr := sc.(*RTSPConnection).targetAddr
	stubChannel, ok := r.stubChannels[stubAddr]

	if !ok {
		return nil, errors.New("can't load stub channel")
	}

	//Send request and waiting for response
	stubChannel.Request <- model.StubChannelRequest{
		model.Data,
		sc.(*RTSPConnection).targetLocal,
		sc.(*RTSPConnection).targetRemote,
		req,
	}
	res := <-stubChannel.Response

	return res.Response, nil
}

func (r *RTSP) getHostFromEndpoint(ep string) (string, error) {
	host, _, err := net.SplitHostPort(ep)
	if err != nil {
		return "", errors.New("could not parse host:port")
	}

	r.log("remote IP to connect: %s", host)

	return host, nil
}

func (r *RTSP) getEndpointFromPath(p *base.URL) (string, error) {
	urls := r.urlHandler.GetInternalURLs(p.String())

	r.log("endpoints to connect: %v", urls)

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
	r.log("endpoint to connect: %s", ep)

	return ep.Host, nil
}

func getRemoteIPv4Address(url string) string {
	res := strings.ReplaceAll(url, "[", "")
	res = strings.ReplaceAll(res, "]", "")
	n := strings.LastIndex(res, ":")

	return fmt.Sprintf("%s", net.ParseIP(res[:n]))
}

func (r *RTSP) isConnectionOpen(ep string) bool {
	r.log("Check RTSP connection for endpoint %s", ep)
	check := false

	r.rtspConn.Range(func(key, value interface{}) bool {
		if value.(*RTSPConnection).targetAddr == ep {
			check = true
		}
		return true
	})
	return check
}

func (r *RTSP) getClientRTSPConnection(connectionKey model.ConnectionKey) (*RTSPConnection, error) {
	// Client RTSPConnection
	rc, ok := r.rtspConn.Load(connectionKey.Key)
	if !ok {
		return nil, errors.New("Can't find client RTSP connection")
	}
	return rc.(*RTSPConnection), nil
}

func (r *RTSP) getRemoteRTSPConnection(connectionKey model.ConnectionKey) (*RTSPConnection, error) {
	// Client RTSPConnection
	rc, err := r.getClientRTSPConnection(connectionKey)
	if err != nil {
		return nil, errors.New("Can't find remote client RTSP connection")
	}

	// Server RTSPConnection
	s_key := model.NewConnectionKey(rc.targetLocal, rc.targetRemote)
	s_rc, s_ok := r.rtspConn.Load(s_key.Key)
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
				// TODO: return 8051 when transport return RTCP
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
				// TODO: return 8051 when transport return RTCP
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
	interleaved := false
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
		r.logError("could not get endpoint from path")
		return url
	}
	// url.Host = fmt.Sprintf("%s:554", ep)
	url.Host = ep
	r.log("Update url to %v", url)
	return url
}
