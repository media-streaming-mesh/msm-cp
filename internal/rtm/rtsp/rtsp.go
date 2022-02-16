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
	"context"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
)

type RTSP struct {
	logger *logrus.Logger
}

// Option configures NewRTSP
type Option func(*options)

// Options configure any protocol
type options struct {
	// Context is the context to use for the transport.
	Context context.Context

	// Logger is the logger to use.
	Logger *logrus.Logger
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

func NewRTSP(opts ...Option) *RTSP {
	var cfg options
	for _, opt := range opts {
		opt(&cfg)
	}
	return &RTSP{
		logger: cfg.Logger,
	}
}

func (r *RTSP) Connect(ctx context.Context, cc *pb.Endpoints) (*emptypb.Empty, error) {
	r.logger.Debugf("Got endpoint Connect: %+v", cc)

	return &emptypb.Empty{}, nil
}

func (r *RTSP) Message(ctx context.Context, cr *pb.Request) (*pb.Response, error) {
	r.logger.Debugf("Got message request: %+v", cr)

	// read request
	req, err := readRequest([]byte(cr.Request))
	if err != nil {
		r.logger.Error("Could not read request: %s", err)
		return nil, err
	}
	return &pb.Response{
		Response: "CP response",
	}, nil
}
