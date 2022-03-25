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
	"context"
	"fmt"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"github.com/sirupsen/logrus"
	"io"
	"strings"
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

func (r *RTSP) Connect(srv pb.MsmControlPlane_ConnectServer) error {
	r.logger.Debugf("start new server")
	ctx := srv.Context()

	for {

		// exit if context is done
		// or continue
		select {
		case <-ctx.Done():
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
		r.logger.Debugf("Got message request: %+v", stream)

		// read request if data
		switch stream.Event {
		case pb.Event_RTSP_DATA:
			rr := bufio.NewReader(strings.NewReader(stream.Message.Data))
			req, err := readRequest(rr)
			if err != nil {
				r.logger.Errorf("could not read request: %s", err)
				return err
			}

			resp := &pb.Response{
				Data: fmt.Sprintf("s", r.handleRequest(req)),
			}

			if err := srv.Send(resp); err != nil {
				r.logger.Errorf("could not send response, error: %v", err)
			}
		default:
		}
	}
}
