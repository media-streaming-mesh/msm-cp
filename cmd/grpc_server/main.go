package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"google.golang.org/grpc/peer"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"

	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedMsmControlPlaneServer
	stubConn  map[string]pb.MsmControlPlane_SendServer
	testCache map[string]string
}

func (s *server) Send(srv pb.MsmControlPlane_SendServer) error {
	ctx := srv.Context()

	fmt.Printf("start new server\n")
	p, _ := peer.FromContext(ctx)
	ip := p.Addr.String()
	s.stubConn[ip] = srv
	s.testCache["test"] = ip

	for {

		// exit if context is done
		// or continue
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// receive data from stream-mapper
		stream, err := srv.Recv()
		if err == io.EOF {
			// return will close stream-mapper from server side
			return nil
		}
		if err != nil {
			continue
		}
		switch stream.Event {
		case pb.Event_REGISTER:
			fmt.Printf("Received REGISTER message request: %+v", stream)
		case pb.Event_ADD:
			fmt.Printf("Received ADD message request: %+v", stream)

		case pb.Event_DELETE:
			fmt.Printf("Received DELETE message request: %+v", stream)

		case pb.Event_DATA:
			fmt.Printf("Received DATA message request: %+v", stream)

		default:
		}
	}

}

func main() {
	// create listener
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// create grpc server
	s := grpc.NewServer()
	srv := &server{
		stubConn:  make(map[string]pb.MsmControlPlane_SendServer),
		testCache: make(map[string]string),
	}

	pb.RegisterMsmControlPlaneServer(s, srv)

	// and start...
	go func() {
		err := s.Serve(lis)
		if err != nil {
			fmt.Printf("Houston problem")
		}
	}()

	time.Sleep(time.Second * 5)
	s2 := srv.testCache["test"]
	fmt.Printf("this is the cache number: %s", s2)
	s1 := srv.stubConn[s2]

	resp := &pb.Message{
		Event:  pb.Event_ADD,
		Local:  "blah",
		Remote: "bloo",
		Data:   "ch",
	}

	if err := s1.Send(resp); err != nil {
		fmt.Println("shiat")
	}
	time.Sleep(time.Second * 10)

}
