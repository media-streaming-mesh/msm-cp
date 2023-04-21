package stream_api

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/media-streaming-mesh/msm-cp/internal/model"
	"github.com/sirupsen/logrus"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"time"
)

var (
	//TODO read endpoint from config
	endpoints      = []string{"192.168.49.3:2379"}
	dialTimeout    = 10 * time.Second
	requestTimeout = 10 * time.Second
)

type StreamAPI struct {
	logger *logrus.Logger
	client *clientv3.Client
}

func NewStreamAPI(logger *logrus.Logger) *StreamAPI {

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})

	if err != nil {
		logger.Errorf("[Stream API] create client error %v", err)
	}

	return &StreamAPI{
		logger: logger,
		client: cli,
	}
}

func (s *StreamAPI) log(format string, args ...interface{}) {
	s.logger.Infof("[Stream API] " + fmt.Sprintf(format, args...))
}

func (s *StreamAPI) logError(format string, args ...interface{}) {
	s.logger.Errorf("[Stream API] " + fmt.Sprintf(format, args...))
}

func (s *StreamAPI) Put(data model.StreamData) error {
	//Prepare data
	//TODO: use protobuf
	jsonData, err := json.Marshal(data)
	stringData := string(jsonData)

	//PUT data
	key := fmt.Sprintf("streamKey:%v", data.ServerIp)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	resp, err := s.client.Put(ctx, key, stringData)
	cancel()
	if err != nil {
		return err
	}
	// use the response
	s.log("PUT response %v", resp)

	return nil
}

func (s *StreamAPI) GetStreams() ([]model.StreamData, error) {
	var streams []model.StreamData
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	resp, err := s.client.Get(ctx, "streamKey", clientv3.WithPrefix())
	cancel()
	if err != nil {
		return streams, err
	}
	s.log("GET response %v", resp)

	for _, response := range resp.Kvs {
		var streamData model.StreamData
		json.Unmarshal(response.Value, &streamData)
		streams = append(streams, streamData)
		s.log("Stream data %v", streamData)
	}
	return streams, nil
}

func (s *StreamAPI) WatchStreams(dataChan chan<- model.StreamData) {
	s.log("Start WATCH")
	watchChan := s.client.Watch(context.Background(), "streamKey", clientv3.WithPrefix())
	for resp := range watchChan {
		for _, event := range resp.Events {
			switch event.Type {
			case mvccpb.PUT:
				// process with put event
				var streamData model.StreamData
				json.Unmarshal(event.Kv.Value, &streamData)
				s.log("Stream data %v", streamData)
				dataChan <- streamData
			case mvccpb.DELETE:
				// process with delete event
			}
		}
	}
}
