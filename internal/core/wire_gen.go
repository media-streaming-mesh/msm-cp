// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//+build !wireinject

package core

import (
	"github.com/media-streaming-mesh/msm-cp/internal/config"
	node_mapper "github.com/media-streaming-mesh/msm-cp/pkg/node-mapper"
)

// Injectors from wire.go:

func InitApp() (*App, error) {
	cfg := config.New()
	grpcImpl := New(cfg)

	nodeMapper := node_mapper.InitializeNodeMapper(cfg)

	app := &App{
		cfg:     cfg,
		grpcImpl: grpcImpl,
		nodeMapper: nodeMapper,
	}
	return app, nil
}
