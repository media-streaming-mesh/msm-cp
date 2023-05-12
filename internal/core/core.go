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

package core

import (
	"context"
	"fmt"
	"github.com/media-streaming-mesh/msm-cp/pkg/config"
	"github.com/media-streaming-mesh/msm-cp/pkg/model"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
	"github.com/media-streaming-mesh/msm-cp/pkg/transport"
	"net"
	"os/signal"
	"syscall"
)

// App contains minimal list of dependencies to be able to start an application.
type App struct {
	cfg *config.Cfg

	grpcImpl   API
	nodeMapper *node_mapper.NodeMapper
	nodeChan   chan model.Node
}

// Start, starts the MSM Control Plane application.
// It will block until the application exits either by:
// 1. cancelling the context set with WithContext
// 2. unrecovered error
func (a *App) Start() error {
	logger := a.cfg.Logger
	logger.Info("Starting MSM Control Plane")

	// Capture signals and block before exit
	ctx, cancel := signal.NotifyContext(context.Background(),
		syscall.SIGHUP,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)
	defer cancel()

	//Watch node
	a.waitForData()
	go func() {
		a.nodeMapper.WatchNode(a.nodeChan)
	}()

	// Listen on a port given from initial config
	grpcPort := fmt.Sprintf("0.0.0.0:%s", a.cfg.Grpc.Port)
	ln, err := net.Listen("tcp", grpcPort)
	if err != nil {
		return err
	}

	transportOptions := []transport.Option{
		transport.UseContext(ctx),
		transport.UseLogger(logger),
		transport.UseListener(ln),
		transport.UseGrpcImpl(a.grpcImpl),
	}

	startTransportErr := make(chan error)

	go func() {
		startTransportErr <- transport.Run(transportOptions...)
	}()

	// block until we exit
	select {
	case err := <-startTransportErr:
		if ctx.Err() != nil {
			logger.Error(err.Error())
			return err
		}
	case <-ctx.Done():
		return nil
	}
	fmt.Println("Exit")
	return nil
}

func (a *App) waitForData() {
	go func() {
		node := <-a.nodeChan
		fmt.Println("Found node %v", node.IP)
		a.waitForData()
	}()
}
