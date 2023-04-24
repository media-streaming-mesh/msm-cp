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
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	msm_url "github.com/media-streaming-mesh/msm-cp/pkg/url-routing/handler"
	"github.com/sirupsen/logrus"
)

type RTSP struct {
	urlHandler   *msm_url.UrlHandler
	logger       *logrus.Logger
	methods      []base.Method
	rtspConn     *sync.Map
	stubChannels map[string]*model.StubChannel
	clientMap    map[string]Client
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
		urlHandler:   uHandler,
		logger:       cfg.Logger,
		methods:      cfg.SupportedMethods,
		rtspConn:     new(sync.Map),
		stubChannels: make(map[string]*model.StubChannel),
		clientMap:    make(map[string]Client),
	}
}

func (r *RTSP) log(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	r.logger.Infof("[RTSP] " + fmt.Sprintf(format, args...))
}

func (r *RTSP) logError(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	r.logger.Debugf("[RTSP] " + fmt.Sprintf(format, args...))
}

// called when a connection is opened.
func (r *RTSP) OnAdd(connectionKey model.ConnectionKey, stubChannels map[string]*model.StubChannel) {
	//Store channel
	r.stubChannels = stubChannels

	// create a new RTSP connection and store it
	rc := newRTSPConnection(r.logger)
	rc.author = connectionKey.Remote

	r.rtspConn.Store(connectionKey.Key, rc)
	r.log("RTSP connection key %v", connectionKey.Key)
	r.log("RTSP connection opened from client %s", connectionKey.Remote)
}

// called when a connection is close.
func (r *RTSP) OnDelete(connectionKey model.ConnectionKey) (*model.StreamData, error) {
	// Get stream data
	streamData := r.getStreamData(connectionKey, model.Teardown)
	// Find RTSP connection and delete it
	r.rtspConn.Delete(connectionKey.Key)

	//Delete client from client map
	delete(r.clientMap, connectionKey.Key)
	r.log("RTSP connection closed from client %s", connectionKey.Remote)

	return streamData, nil
}

func (r *RTSP) OnData(conn pb.MsmControlPlane_SendServer, stream *pb.Message) (*model.StreamData, error) {
	//Read stream data
	connectionKey := model.NewConnectionKey(stream.Local, stream.Remote)
	var streamData *model.StreamData
	var buffer = bytes.NewBuffer(make([]byte, 0, 4096))

	reqReader := bufio.NewReader(strings.NewReader(stream.Data))
	resReader := bufio.NewReader(strings.NewReader(stream.Data))

	req := &base.Request{}
	res := &base.Response{}

	errReq := req.Read(reqReader)
	errRes := res.Read(resReader)

	if errReq != nil && errRes != nil {
		return nil, fmt.Errorf("request error=%s response error=%s", errRes, errRes)
	} else if errReq == nil {
		// Get client ports
		clientPorts := getClientPorts(req.Header["Transport"])
		if len(clientPorts) != 0 {
			client := r.clientMap[connectionKey.Key]
			r.clientMap[connectionKey.Key] = Client{
				clientIp:    client.clientIp,
				clientPorts: clientPorts,
				serverIp:    client.serverIp,
			}
		}

		if req.Method == base.Teardown {
			streamData = r.getStreamData(connectionKey, model.Teardown)
		}
		// received a client-side request
		pbMsg, err := r.handleRequest(req, connectionKey)
		if err != nil {
			return nil, fmt.Errorf("handle request error=%s", err)
		}
		pbMsg.Write(buffer)
		pbRes := &pb.Message{
			Event:  stream.Event,
			Local:  stream.Local,
			Remote: stream.Remote,
			Data:   fmt.Sprintf("%s", buffer),
		}

		r.log("response to client is %v", pbRes)

		// Send response back to client
		err = conn.Send(pbRes)
		if err != nil {
			return nil, fmt.Errorf("send response error=%s", err)
		}

		// Get stream data
		if req.Method == base.Setup {
			streamData = r.getStreamData(connectionKey, model.Create)
		}
		if req.Method == base.Play {
			streamData = r.getStreamData(connectionKey, model.Play)
		}
	} else if errRes == nil {
		// received a server-side response
		err := r.handleResponse(res, connectionKey)
		if err != nil {
			return nil, fmt.Errorf("handle response error=%s", err)
		}
	}
	return streamData, nil
}

func (r *RTSP) getStreamData(connectionKey model.ConnectionKey, streamState model.StreamState) *model.StreamData {
	rc, err := r.getClientRTSPConnection(connectionKey)
	if err != nil {
		return nil
	}

	s_rc, err := r.getRemoteRTSPConnection(connectionKey)
	if err != nil {
		return nil
	}

	serverAddress := getRemoteIPv4Address(rc.targetRemote)
	clientAddress := getRemoteIPv4Address(connectionKey.Remote)
	serverPorts := getServerPorts(s_rc.response[Setup].Header["Transport"])
	client := r.clientMap[connectionKey.Key]

	r.log("server address/ports %v %v", serverAddress, serverPorts)
	r.log("client address/ports %v %v", clientAddress, client.clientPorts)

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
