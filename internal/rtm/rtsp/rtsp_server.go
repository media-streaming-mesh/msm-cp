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
	"strconv"
	"strings"
	"sync"

	"github.com/media-streaming-mesh/msm-cp/internal/transport"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"

	"github.com/aler9/gortsplib/pkg/base"
	"github.com/sirupsen/logrus"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
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
			r.OnConnOpen(stream)

		case pb.Event_DELETE:
			r.logger.Debugf("Received DELETE event: %v", stream)
			r.OnConnClose(stream)

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

				// this is a total hack!   Need to pass something more than just RTSP messages around (e.g. IPs/ports/etc.)
				// grab client port
				hdr := req.Header["Transport"][0]
				ports := strings.Split(hdr, "=")[1]
				port, _ := strconv.ParseUint(strings.Split(ports, "-")[0], 10, 32)
				r.logger.Debugf("Hack port %v", port)

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
	serverEp := getRemoteIPv4Address(rc.targetRemote)

	r.logger.Debugf("client EP is %v", clientEp)
	r.logger.Debugf("server EP is %v", serverEp)

	dataplaneIP, err := node_mapper.MapNode(clientEp)

	if err != nil {
		nodeEp := getRemoteIPv4Address(s.Local)

		r.logger.Debugf("node EP is %v", nodeEp)

		dataplaneIP, err = node_mapper.MapNode(nodeEp)
	}

	r.logger.Debugf("msm-proxy ip %v", dataplaneIP)

	if err != nil {
		return err
	}

	// // 3. Get client/remote ports
	// these need to be assigned by controller - not stub or app
	describeResponse := s_rc.response[Setup]
	clientPorts := getClientPorts(describeResponse.Header["Transport"])
	serverPorts := getServerPorts(describeResponse.Header["Transport"])

	r.logger.Debugf("client endpoint/ports %v %v", clientEp, clientPorts)
	r.logger.Debugf("server endpoint/ports %v %v", serverEp, serverPorts)

	//TODO: create GRPC connection to server once
	grpcClient, err := transport.SetupClient(dataplaneIP)
	if err != nil {
		r.logger.Debugf("Failed to connect to server, error %s\n", err)
	}
	dpGrpcClient := transport.Client{
		r.logger,
		grpcClient,
	}

	if rc.state == Setup {
		var streamId uint32

		data, ok := r.rtspStream.Load(serverEp)
		if ok {
			streamId = data.(uint32)
		} else {
			// need to add ports for multiple sessions
			stream, result := dpGrpcClient.CreateStream(serverEp, serverPorts[0])
			streamId = stream.Id
			r.rtspStream.Store(serverEp, streamId)
			r.logger.Debugf("Create stream %v %v", stream, result)
		}

		endpoint, result := dpGrpcClient.CreateEndpoint(streamId, clientEp, clientPorts[0])
		r.rtspEndpoint.Store(clientEp, streamId)
		r.logger.Debugf("Created ep %v %v", endpoint, result)
	}

	if rc.state == Play {
		streamId, ok := r.rtspStream.Load(serverEp)
		if !ok {
			return errors.New("Can't find stream id")
		}
		endpoint, result := dpGrpcClient.UpdateEndpoint(streamId.(uint32), clientEp, clientPorts[0])
		r.logger.Debugf("Update ep %v %v", endpoint, result)
	}

	if rc.state == Teardown {
		streamId, ok := r.rtspStream.Load(serverEp)
		if !ok {
			return errors.New("Can't find stream id")
		}
		endpoint, result := dpGrpcClient.DeleteEndpoint(streamId.(uint32), clientEp, clientPorts[0])
		r.logger.Debugf("Delete ep %v %v", endpoint, result)

		if r.isLastClient(clientEp) {
			stream, result := dpGrpcClient.DeleteStream(streamId.(uint32), serverEp, 8050)
			r.rtspStream.Delete(serverEp)
			r.logger.Debugf("Delete stream %v %v", stream, result)
		}
		r.rtspEndpoint.Delete(clientEp)
	}

	dpGrpcClient.Close()

	return nil
}
