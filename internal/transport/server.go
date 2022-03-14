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
	"context"
	"net"
	"sync"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/sirupsen/logrus"
)

// Server holds the server specific data structures
type Server struct {
	log logrus.Logger

	grpcServer *grpcServer
	httpServer *httpServer
	vclServer  *vclServer
}

// Option configures Run
type Option func(*options)

// options configure the transport
// options are passed through run and normally are set with flags
// or a configuration file before the start of the MSM control plane
type options struct {
	// Context is the context to use for the transport.
	Context context.Context

	// Logger is the logger to use.
	Logger *logrus.Logger

	// GRPCListener sets up the gRPC server
	GrpcListener net.Listener

	// Impl is the backend implementation to use for the grpc transport
	GrpcImpl pb.MsmControlPlaneServer
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

// UseListener sets the GRPC listener
func UseListener(ln net.Listener) Option {
	return func(opts *options) { opts.GrpcListener = ln }
}

// UseImpl sets the grpc implementation to serve
func UseImpl(impl pb.MsmControlPlaneServer) Option {
	return func(opts *options) {
		opts.GrpcImpl = impl
	}
}

func Run(opts ...Option) error {
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}

	log := cfg.Logger

	grpcServer, err := newGrpcServer(&cfg)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}

	var gprcErrs = make(chan error, 1)
	wg.Add(1)
	go func() {
		err := grpcServer.start()
		gprcErrs <- err
		log.Debug("GRPC server has exited", "err", err)
		wg.Done()
	}()

	ctx, cancel := context.WithCancel(cfg.Context)
	defer cancel()
	defer wg.Wait() // Wait for server run processes to exit before returning

	select {
	case err := <-gprcErrs:
		log.Error("failed to run the GRPC server", "err", err)
		return err
	case <-cfg.Context.Done():
		grpcServer.close()
		return ctx.Err()
	}
}
