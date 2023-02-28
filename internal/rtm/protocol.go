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
	"fmt"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/media-streaming-mesh/msm-cp/internal/rtm/rtsp"
	"github.com/media-streaming-mesh/msm-cp/internal/stub"
	stream_mapper "github.com/media-streaming-mesh/msm-cp/pkg/stream-mapper"
	"github.com/sirupsen/logrus"
	"io"
	"sync"
)

// API provides external access to
type API interface {
	Send(conn pb.MsmControlPlane_SendServer) error
}

// Protocol holds the rtm protocol specific data structures
type Protocol struct {
	cfg         *config.Cfg
	logger      *logrus.Logger
	stubHandler *stub.StubHandler
	//TODO: move stream_mapper to msm-nc
	streamMapper *stream_mapper.StreamMapper

	//rtm protocol
	rtsp *rtsp.RTSP
}

func New(cfg *config.Cfg) *Protocol {
	ctx := context.Background()

	rtspOpts := []rtsp.Option{
		rtsp.UseContext(ctx),
		rtsp.UseLogger(cfg.Logger),
		rtsp.UseMethods(cfg.SupportedMethods),
	}

	return &Protocol{
		cfg:          cfg,
		logger:       cfg.Logger,
		stubHandler:  stub.NewStubHandler(cfg),
		streamMapper: stream_mapper.NewStreamMapper(cfg.Logger, new(sync.Map)),
		rtsp:         rtsp.NewRTSP(rtspOpts...),
	}
}

func (p *Protocol) log(format string, args ...interface{}) {
	p.logger.Infof("[RTM] " + fmt.Sprintf(format, args...))
}

func (p *Protocol) logError(format string, args ...interface{}) {
	p.logger.Errorf("[RTM] " + fmt.Sprintf(format, args...))
}

func (p *Protocol) Send(conn pb.MsmControlPlane_SendServer) error {
	var ctx = conn.Context()
	for {
		// exit if context is done or continue
		select {
		case <-ctx.Done():
			p.log("received connection done")
			return nil
		default:
		}

		//Process stream data
		stream, err := conn.Recv()
		if err == io.EOF {
			// return will close stream-mapper from server side
			p.logError("found EOF, exiting")
			return nil
		}
		if err != nil {
			p.logError("received error %v", err)
			continue
		}

		switch stream.Event {
		case pb.Event_REGISTER:
			p.log("Received REGISTER event: %v", stream)
		case pb.Event_ADD:
			p.log("Received ADD event: %v", stream)
		case pb.Event_DELETE:
			p.log("Received DELETE event: %v", stream)
		case pb.Event_DATA:
			p.log("Received DATA event: %v", stream)
		default:
		}

		//Handle rtm request
		var streamData *model.StreamData
		proto := p.cfg.Protocol
		switch proto {
		case "rtsp":
			streamData, err = p.rtsp.Send(conn, stream)
		default:
		}
		if err != nil {
			return err
		}

		//Handle stub request
		p.stubHandler.Send(conn, stream)

		//Send data to proxy
		if streamData != nil {
			//TODO: Writes logical stream graphs to etcd cluster
			stubAddress := stub.GetStubAddress(streamData.ClientIp, stream.Remote)
			p.log("StubAddress %v", stubAddress)

			error := p.streamMapper.ProcessStream(model.StreamData{
				StubIp:      stubAddress,
				ServerIp:    streamData.ServerIp,
				ClientIp:    streamData.ClientIp,
				ServerPorts: streamData.ServerPorts,
				ClientPorts: streamData.ClientPorts,
				StreamState: streamData.StreamState,
			})

			if error != nil {
				p.logError("ProcessStream failed %v", error)
			}

		}
	}
}
