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

	"github.com/sirupsen/logrus"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
)

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

// UseGrpcImpl sets the grpc implementation to serve
func UseGrpcImpl(impl pb.MsmControlPlaneServer) Option {
	return func(opts *options) {
		opts.GrpcImpl = impl
	}
}
