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
	"errors"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/rtm/rtsp"
)

// API provides external access to
type API interface {
	Send(srv pb.MsmControlPlane_SendServer) error
}

// Protocol holds the rtm protocol specific data structures
type Protocol struct {
	cfg *config.Cfg

	rtsp *rtsp.RTSP
}

func New(cfg *config.Cfg) *Protocol {
	ctx := context.Background()

	rtspOpts := []rtsp.Option{
		rtsp.UseContext(ctx),
		rtsp.UseLogger(cfg.Logger),
		rtsp.UseRemote(cfg.Remote),
		rtsp.UseMethods(cfg.SupportedMethods),
	}

	return &Protocol{
		cfg:  cfg,
		rtsp: rtsp.NewRTSP(rtspOpts...),
	}
}

func (p *Protocol) Send(srv pb.MsmControlPlane_SendServer) error {
	proto := p.cfg.Protocol

	switch proto {
	case "rtsp":
		return p.rtsp.Send(srv)
	default:
		return errors.New("failed to get rtm protocol type from config")
	}
}
