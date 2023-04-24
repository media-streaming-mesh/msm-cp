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
	"sync"

	"github.com/sirupsen/logrus"
)

// Server holds the server specific data structures
type Server struct {
	log logrus.Logger

	grpcServer *grpcServer
	httpServer *httpServer
	vclServer  *vclServer
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
	log.Infof("GRPC server created")

	wg := sync.WaitGroup{}

	gprcErrs := make(chan error, 1)
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
		log.Infof("GRPC server done")
		grpcServer.close()
		return ctx.Err()
	}
}
