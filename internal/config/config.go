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

// Package config
package config

import (
	"flag"

	"github.com/aler9/gortsplib/pkg/base"
	"github.com/sirupsen/logrus"
)

// Config holds the configuration data for the MSM control plane
// application
type Cfg struct {
	Dataplane        string
	Protocol         string
	Remote           string
	Logger           *logrus.Logger
	Grpc             *grpcOpts
	SupportedMethods []base.Method
}

type grpcOpts struct {
	Port string
}

// New initializes the configuration plugin and is shared across
// the internal plugins via the API interface
func New() *Cfg {
	cf := new(Cfg)
	grpcOpt := new(grpcOpts)

	flag.StringVar(&cf.Dataplane, "dataplane", "msm", "dataplane to connect to (msm, vpp)")
	flag.StringVar(&grpcOpt.Port, "grpcPort", "9000", "port to listen for GRPC on")
	flag.StringVar(&cf.Protocol, "protocol", "rtsp", "control plane protocol mode (rtsp, rist)")
	logLevel := flag.Int("loglevel", int(logrus.InfoLevel), "Log level")

	flag.Parse()

	cf.Logger = logrus.New()

	cf.Logger.SetFormatter(&logrus.TextFormatter{
		ForceColors:     true,
		DisableColors:   false,
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05.123",
	})

	cf.Logger.Level = logrus.Level(*logLevel)

	return &Cfg{
		Dataplane: cf.Dataplane,
		Protocol:  cf.Protocol,
		Logger:    cf.Logger,
		Remote:    cf.Remote,
		Grpc: &grpcOpts{
			Port: grpcOpt.Port,
		},
	}
}
