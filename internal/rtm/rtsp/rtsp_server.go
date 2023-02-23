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
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"strings"
	"sync"

	"github.com/aler9/gortsplib/pkg/base"
	"github.com/sirupsen/logrus"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	msm_url "github.com/media-streaming-mesh/msm-cp/pkg/url-routing/handler"
)

type RTSP struct {
	urlHandler *msm_url.UrlHandler
	logger     *logrus.Logger
	methods    []base.Method
	rtspConn   *sync.Map
	clientMap  map[string]Client
}

type Client struct {
	clientIp    string
	clientPorts []uint32
	serverIp    string
}

// Option configures NewRTSP
type Option func(*options)

// Options configure any protocol
type options struct {
	// Context is the context to use for the transport.
	Context context.Context

	// Logger is the logger to use.
	Logger *logrus.Logger

	// RTSP supported Methods
	SupportedMethods []base.Method
}

// UseContext sets the context for the server
func UseContext(ctx context.Context) Option {
	return func(opts *options) {
		opts.Context = ctx
	}
}

// UseLogger sets the logger
func UseLogger(log *logrus.Logger) Option {
	return func(opts *options) {
		opts.Logger = log
	}
}

// UseMethods sets the server's available methods
func UseMethods(m []base.Method) Option {
	return func(opts *options) {
		opts.SupportedMethods = m
	}
}

func NewRTSP(opts ...Option) *RTSP {
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}
	uHandler := &msm_url.UrlHandler{}
	uHandler.InitializeUrlHandler()

	return &RTSP{
		urlHandler: uHandler,
		logger:     cfg.Logger,
		methods:    cfg.SupportedMethods,
		rtspConn:   new(sync.Map),
		clientMap:  make(map[string]Client),
	}

}

// called when a connection is opened.
func (r *RTSP) OnConnOpen(stream *pb.Message) {
	// create a new RTSP connection and store it
	rc := newRTSPConnection(r.logger)
	rc.author = stream.Remote

	key := getRTSPConnectionKey(stream.Local, stream.Remote)
	r.rtspConn.Store(key, rc)
	r.logger.Infof("RTSP connection key %v", key)
	r.logger.Infof("RTSP connection opened from client %s", stream.Remote)
}

// called when a connection is close.
func (r *RTSP) OnConnClose(stream *pb.Message) *model.StreamData {
	// Get stream data
	streamData := r.getStreamData(stream, model.Teardown)
	// Find RTSP connection and delete it
	key := getRTSPConnectionKey(stream.Local, stream.Remote)
	r.rtspConn.Delete(key)

	//Delete client from client map
	connectionKey := getRTSPConnectionKey(stream.Local, stream.Remote)
	delete(r.clientMap, connectionKey)

	r.logger.Infof("RTSP connection closed from client %s", stream.Remote)

	return streamData
}

func (r *RTSP) OnExternalClientConnOpen(stream *pb.Message, stubData pb.Message) {
	// create a new RTSP connection and store it
	rc := newRTSPConnection(r.logger)
	rc.author = stream.Remote
	rc.targetLocal = stubData.Local
	rc.targetRemote = stubData.Remote

	key := getRTSPConnectionKey(stream.Local, stream.Remote)
	r.rtspConn.Store(key, rc)
	r.logger.Infof("RTSP connection key %v", key)
	r.logger.Infof("RTSP connection opened from external client %s", stream.Remote)
}

func (r *RTSP) OnData(conn pb.MsmControlPlane_SendServer, stream *pb.Message) *model.StreamData {
	//Read stream data
	var streamData *model.StreamData
	var data = bytes.NewBuffer(make([]byte, 0, 4096))
	reqReader := bufio.NewReader(strings.NewReader(stream.Data))
	resReader := bufio.NewReader(strings.NewReader(stream.Data))

	req := &base.Request{}
	res := &base.Response{}

	errReq := req.Read(reqReader)
	errRes := res.Read(resReader)

	if errReq != nil && errRes != nil {
		r.logger.Errorf("[RTSP] request error=%s", errReq)
		r.logger.Errorf("[RTSP] response error=%s", errRes)
	} else if errReq == nil {
		//Get client ports
		clientPorts := getClientPorts(req.Header["Transport"])
		if len(clientPorts) != 0 {
			connectionKey := getRTSPConnectionKey(stream.Local, stream.Remote)
			client := r.clientMap[connectionKey]
			r.clientMap[connectionKey] = Client{
				clientIp:    client.clientIp,
				clientPorts: clientPorts,
				serverIp:    client.serverIp,
			}
		}

		if req.Method == base.Teardown {
			streamData = r.getStreamData(stream, model.Teardown)
		}
		// received a client-side request
		pbMsg, err := r.handleRequest(req, stream)
		if err != nil {
			r.logger.Errorf("[RTSP] handle request error=%s", err)
			return nil
		}
		pbMsg.Write(data)
		pbRes := &pb.Message{
			Event:  stream.Event,
			Local:  stream.Local,
			Remote: stream.Remote,
			Data:   fmt.Sprintf("%s", data),
		}

		r.logger.Debugf("[RTSP] response to client is %v", pbRes)

		//Send response back to client
		err = conn.Send(pbRes)
		if err != nil {
			r.logger.Errorf("[RTSP] could not send response, error: %v", err)
			return nil
		}

		//Get stream data
		if req.Method == base.Setup {
			streamData = r.getStreamData(stream, model.Create)
		}
		if req.Method == base.Play {
			streamData = r.getStreamData(stream, model.Play)
		}

	} else if errRes == nil {
		// received a server-side response
		err := r.handleResponse(res, stream)
		if err != nil {
			r.logger.Errorf("[RTSP] incoming request error=%s", err)
			return nil
		}
	}
	return streamData
}

func (r *RTSP) getStreamData(stream *pb.Message, streamState model.StreamState) *model.StreamData {
	rc, err := r.getClientRTSPConnection(stream)
	if err != nil {
		return nil
	}

	s_rc, err := r.getRemoteRTSPConnection(stream)
	if err != nil {
		return nil
	}

	serverAddress := getRemoteIPv4Address(rc.targetRemote)
	clientAddress := getRemoteIPv4Address(stream.Remote)
	serverPorts := getServerPorts(s_rc.response[Setup].Header["Transport"])
	client := r.clientMap[getRTSPConnectionKey(stream.Local, stream.Remote)]

	r.logger.Debugf("[RTSP] server address/ports %v %v", serverAddress, serverPorts)
	r.logger.Debugf("[RTSP] client address/ports %v %v", clientAddress, client.clientPorts)

	streamData := model.StreamData{
		"",
		serverAddress,
		clientAddress,
		serverPorts,
		client.clientPorts,
		streamState,
	}

	return &streamData
}
