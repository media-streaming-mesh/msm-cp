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

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/endpoint"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
)

type API interface {
	GetEndpoint(ctx context.Context, in *pb.EndpointRequest) (*pb.EndpointResponse, error)
}

type Protocol struct {
	cfg *config.Cfg

	rtsp *RTSP
}

func New(cfg *config.Cfg) *Protocol {
	return &Protocol{
		cfg:  cfg,
		rtsp: NewRTSP(),
	}
}

func (p *Protocol) GetEndpoint(ctx context.Context, in *pb.EndpointRequest) (*pb.EndpointResponse, error) {
	proto := p.cfg.Protocol
	res := &pb.EndpointResponse{}

	switch proto {
	case "rtsp":
		var err error
		res, err = p.rtsp.GetEndpoint(ctx, in)
		if err != nil {
			return res, err
		}
	}

	return res, nil
}
