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
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/media-streaming-mesh/msm-cp/internal/transport"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"

	"github.com/aler9/gortsplib/pkg/base"
	"github.com/sirupsen/logrus"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	pb_dp "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_dp"
	msm_url "github.com/media-streaming-mesh/msm-cp/pkg/url-routing/handler"
)

type RTSP struct {
	urlHandler   *msm_url.UrlHandler
	logger       *logrus.Logger
	methods      []base.Method
	stubConn     *sync.Map
	rtspConn     *sync.Map
	rtspStream   *sync.Map
	rtspEndpoint *sync.Map
	rtpPort      *sync.Map
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
		stubConn:     new(sync.Map),
		rtspConn:     new(sync.Map),
		rtspStream:   new(sync.Map),
		rtspEndpoint: new(sync.Map),
		rtpPort:      new(sync.Map),
	}

}

func (r *RTSP) Send(srv pb.MsmControlPlane_SendServer) error {
	var ctx = srv.Context()
	var data = bytes.NewBuffer(make([]byte, 0, 4096))

	for {

		// exit if context is done or continue
		select {
		case <-ctx.Done():
			r.logger.Debugf("reveiced connection done")
			return ctx.Err()
		default:
		}

		// receive data from stream
		stream, err := srv.Recv()
		if err == io.EOF {
			// return will close stream from server side
			r.logger.Info("found EOF, exiting")
			return nil
		}
		if err != nil {
			r.logger.Errorf("received error %v", err)
			continue
		}

		switch stream.Event {
		case pb.Event_REGISTER:
			r.logger.Debugf("Received REGISTER event: %v", stream)
			r.OnRegistration(srv)

		case pb.Event_ADD:
			r.logger.Debugf("Received ADD event: %v", stream)
			r.OnConnOpen(srv, stream)

		case pb.Event_DELETE:
			r.logger.Debugf("Received DELETE event: %v", stream)
			r.OnConnClose(srv, stream)

		case pb.Event_DATA:
			r.logger.Debugf("Received DATA event: %v", stream)

			reqReader := bufio.NewReader(strings.NewReader(stream.Data))
			resReader := bufio.NewReader(strings.NewReader(stream.Data))

			req := &base.Request{}
			res := &base.Response{}

			errReq := req.Read(reqReader)
			errRes := res.Read(resReader)

			if errReq != nil && errRes != nil {
				return err
			} else if errReq == nil {
				// received a client-side request

				pbMsg, err := r.handleRequest(req, stream)
				if err != nil {
					r.logger.Errorf("incoming request error=%s", err)
					return err
				}
				pbMsg.Write(data)
				pbRes := &pb.Message{
					Event:  stream.Event,
					Local:  stream.Local,
					Remote: stream.Remote,
					Data:   fmt.Sprintf("%s", data),
				}

				//Send response back to client
				if err := srv.Send(pbRes); err != nil {
					r.logger.Errorf("could not send response, error: %v", err)
				} else {
					//Update data to proxy
					if req.Method == base.Setup || req.Method == base.Play {
						if err := r.SendProxyData(stream); err != nil {
							r.logger.Errorf("Could not send proxy data %v", err)
						}
					}
				}
			} else if errRes == nil {
				// received a server-side response
				err := r.handleResponse(res, stream)
				if err != nil {
					r.logger.Errorf("incoming request error=%s", err)
					return err
				}
			}

		default:
		}
	}
}

func (r *RTSP) SendProxyData(s *pb.Message) error {

	var serverProxyIP string
	var clientProxyIP string
	var serverDpGrpcClient transport.Client
	var clientDpGrpcClient transport.Client

	// 1. Get client/remote RTSP connection
	rc, err := r.getClientRTSPConnection(s)
	if err != nil {
		return err
	}

	s_rc, err := r.getRemoteRTSPConnection(s)
	if err != nil {
		return err
	}

	// 2. Get client/remote endpoint
	clientEp := getRemoteIPv4Address(s.Remote)
	clientPort, _ := r.rtpPort.Load(clientEp)
	serverEp := getRemoteIPv4Address(rc.targetRemote)

	r.logger.Debugf("client EP is %v", clientEp)
	r.logger.Debugf("server EP is %v", serverEp)

	//Check if client/server on same node
	isOnSameNode := node_mapper.IsOnSameNode(clientEp, serverEp)

	clientProxyIP, err = node_mapper.MapNode(clientEp)
	if err != nil {
		nodeEp := getRemoteIPv4Address(s.Local)

		r.logger.Debugf("node EP is %v", nodeEp)

		clientProxyIP, err = node_mapper.MapNode(nodeEp)
	}
	r.logger.Debugf("client msm-proxy ip %v", clientProxyIP)

	if err != nil {
		return err
	}

	if !isOnSameNode {
		serverProxyIP, err = node_mapper.MapNode(serverEp)
		if err != nil {
			return err
		}
		r.logger.Debugf("server msm-proxy ip %v", serverProxyIP)
	}

	//TODO: create GRPC connection to server once
	clientGrpcClient, err := transport.SetupClient(clientProxyIP)
	if err != nil {
		r.logger.Debugf("Failed to setup GRPC client, error %s\n", err)
	}
	clientDpGrpcClient = transport.Client{
		r.logger,
		clientGrpcClient,
	}

	if !isOnSameNode {
		serverGrpcClient, err := transport.SetupClient(serverProxyIP)
		if err != nil {
			r.logger.Debugf("Failed to setup GRPC client, error %s\n", err)
		}
		serverDpGrpcClient = transport.Client{
			r.logger,
			serverGrpcClient,
		}
	}

	if rc.state == Setup {
		var streamId uint32

		// // 3. Get client/remote ports
		// these need to be assigned by controller - not stub or app
		describeResponse := s_rc.response[Setup]
		serverPorts := getServerPorts(describeResponse.Header["Transport"])

		r.logger.Debugf("client endpoint/ports %v %v", clientEp, clientPort)
		r.logger.Debugf("server endpoint/ports %v %v", serverEp, serverPorts)

		data, ok := r.rtspStream.Load(serverEp)
		if ok {
			streamId = data.(uint32)
		} else {
			streamId = transport.GetStreamID()
			if isOnSameNode {
				stream, result := clientDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, serverEp, serverPorts[0])
				streamId = stream.Id
				r.rtspStream.Store(serverEp, streamId)
				r.logger.Debugf("Create stream %v result %v", stream, result)
			} else {
				stream, result := serverDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, serverEp, serverPorts[0])
				streamId = stream.Id
				r.rtspStream.Store(serverEp, streamId)
				r.logger.Debugf("Create stream %v result %v", stream, result)

				stream2, result := clientDpGrpcClient.CreateStream(streamId, pb_dp.Encap_RTP_UDP, serverProxyIP, serverPorts[0])
				r.logger.Debugf("Create stream2 %v result %v", stream2, result)
				r.logger.Debugf("stream2 ep encap %v", stream2.Endpoint.Encap)

			}
		}

		endpoint, result := clientDpGrpcClient.CreateEndpoint(streamId, pb_dp.Encap_RTP_UDP, clientEp, clientPort.(uint32))
		r.rtspEndpoint.Store(clientEp, streamId)
		r.logger.Debugf("Created ep %v result %v", endpoint, result)

		if !isOnSameNode {
			endpoint, result := serverDpGrpcClient.CreateEndpoint(streamId, pb_dp.Encap_RTP_UDP, clientProxyIP, clientPort.(uint32))
			r.rtspEndpoint.Store(clientEp, streamId)
			r.logger.Debugf("Created ep %v result %v", endpoint, result)
			r.logger.Debugf("Endpoint encap %v", endpoint.Encap)
		}
	}

	if rc.state == Play {
		streamId, ok := r.rtspStream.Load(serverEp)
		if !ok {
			return errors.New("Can't find stream id")
		}

		endpoint, result := clientDpGrpcClient.UpdateEndpoint(streamId.(uint32), clientEp, clientPort.(uint32))
		r.logger.Debugf("Update ep %v %v", endpoint, result)

		if !isOnSameNode {
			endpoint2, result := serverDpGrpcClient.UpdateEndpoint(streamId.(uint32), clientProxyIP, clientPort.(uint32))
			r.logger.Debugf("Update ep %v %v", endpoint2, result)
		}
	}

	if rc.state == Teardown {
		streamId, ok := r.rtspStream.Load(serverEp)
		if !ok {
			return errors.New("Can't find stream id")
		}

		clientPort, _ := r.rtpPort.Load(clientEp)

		endpoint, result := clientDpGrpcClient.DeleteEndpoint(streamId.(uint32), clientEp, clientPort.(uint32))
		r.logger.Debugf("Delete ep %v %v", endpoint, result)

		if !isOnSameNode {
			endpoint2, result := serverDpGrpcClient.DeleteEndpoint(streamId.(uint32), clientProxyIP, clientPort.(uint32))
			r.logger.Debugf("Delete ep %v %v", endpoint2, result)
		}

		if r.isLastClient(clientEp) {
			if isOnSameNode {
				stream, result := clientDpGrpcClient.DeleteStream(streamId.(uint32), serverEp, 8050)
				r.rtspStream.Delete(serverEp)
				r.logger.Debugf("Delete stream %v %v", stream, result)
			} else {
				stream, result := serverDpGrpcClient.DeleteStream(streamId.(uint32), serverEp, 8050)
				r.logger.Debugf("Delete stream %v %v", stream, result)

				stream2, result := clientDpGrpcClient.DeleteStream(streamId.(uint32), serverProxyIP, 8050)
				r.logger.Debugf("Delete stream %v %v", stream2, result)
			}

		}
		r.rtspEndpoint.Delete(clientEp)
		r.rtpPort.Delete(clientEp)
	}

	clientDpGrpcClient.Close()
	if !isOnSameNode {
		serverDpGrpcClient.Close()
	}

	return nil
}
