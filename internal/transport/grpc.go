/*
 * Copyright (c) 2022 Cisco and/or its affiliates.
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

package transport

import (
	"time"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/endpoint"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
)

type grpcServer struct {
	opts *options

	server *grpc.Server
}

// newGrpcServer initializes a new gRPC server
func newGrpcServer(opts *options) (*grpcServer, error) {

	var optsArr []grpc.ServerOption
	optsArr = append(optsArr,
		grpc.ChainUnaryInterceptor(),
		grpc.ChainStreamInterceptor(),
		grpc.KeepaliveEnforcementPolicy(
			keepalive.EnforcementPolicy{
				MinTime:             20 * time.Second,
				PermitWithoutStream: true,
			}),
	)

	s := grpc.NewServer(optsArr...)

	return &grpcServer{
		opts:   opts,
		server: s,
	}, nil
}

func (s *grpcServer) start() error {
	pb.RegisterEndpointServer(s.server, s.opts.Impl)
	l := s.opts.GrpcListener
	s.opts.Logger.Info("starting gRPC server", "addr", l.Addr().String())
	return s.server.Serve(l)
}

// close gracefully stops the grpc server
func (s *grpcServer) close() {
	log := s.opts.Logger

	// Graceful in a goroutine so we can timeout
	graceCh := make(chan struct{})
	go func() {
		defer close(graceCh)
		log.Debug("gracefully stopping grpc server")
		s.server.GracefulStop()
	}()

	select {
	case <-graceCh:
		log.Debug("gracefully stopped grpc server")

	case <-time.After(5 * time.Second):
		log.Debug("forcefully stopping after 5 seconds of wait")
		s.server.Stop()
	}
}
