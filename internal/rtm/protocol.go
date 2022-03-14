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

package rtm

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/rtm/rtsp"
	"github.com/sirupsen/logrus"
)

// API provides external access to
type API interface {
	ClientConnect(ctx context.Context, cc *pb.Endpoints) (*emptypb.Empty, error)
	ClientRequest(ctx context.Context, cr *pb.Request) (*pb.Response, error)
	ServerConnect(ctx context.Context, cc *pb.Endpoints) (*emptypb.Empty, error)
	ServerRequest(ctx context.Context, cr *pb.Request) (*pb.Response, error)
}

func (p *Protocol) ServerConnect(ctx context.Context, cc *pb.Endpoints) (*emptypb.Empty, error) {
	//TODO implement me
	panic("implement me")
}

func (p *Protocol) ServerRequest(ctx context.Context, cr *pb.Request) (*pb.Response, error) {
	//TODO implement me
	panic("implement me")
}

// Protocol holds the rtm protocol specific data structures
type Protocol struct {
	cfg *config.Cfg

	rtsp *rtsp.RTSP
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
}

func New(cfg *config.Cfg) *Protocol {
	return &Protocol{
		cfg:  cfg,
		rtsp: rtsp.NewRTSP(),
	}
}

func (p *Protocol) ClientConnect(ctx context.Context, cc *pb.Endpoints) (*emptypb.Empty, error) {
	proto := p.cfg.Protocol
	res := &emptypb.Empty{}

	switch proto {
	case "rtsp":
		var err error
		res, err = p.rtsp.Connect(ctx, cc)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}

func (p *Protocol) ClientRequest(ctx context.Context, cr *pb.Request) (*pb.Response, error) {
	proto := p.cfg.Protocol
	res := &pb.Response{}

	switch proto {
	case "rtsp":
		var err error
		res, err = p.rtsp.Message(ctx, cr)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}
