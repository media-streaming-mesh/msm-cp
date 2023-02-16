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

package main

import (
	"context"
	"fmt"
	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"google.golang.org/grpc"
	"io"
	"log"
	"os"
)

func main() {
	fmt.Printf("Starting grpc client...\n")

	conn, err := grpc.Dial("127.0.0.1:9000", grpc.WithInsecure())
	if err != nil {
		fmt.Printf("Failed to connect to server, error %s\n", err)
		fmt.Printf("Exiting...\n")
		os.Exit(1)
	}
	defer conn.Close()

	client := pb.NewMsmControlPlaneClient(conn)
	stream, err := client.Send(context.Background())
	if err != nil {
		log.Fatalf("openn stream-mapper error %v", err)
	}

	ctx := stream.Context()
	done := make(chan bool)

	req := pb.Message{
		Event: pb.Event_REGISTER,
	}
	if err := stream.Send(&req); err != nil {
		log.Fatalf("can not send %v", err)
	}

	// receive from server on a separate goroutine
	go func() {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				close(done)
				return
			}
			if err != nil {
				log.Fatalf("can not receive %v", err)
			}
			log.Printf("response received: %v", resp)
		}
	}()

	go func() {
		<-ctx.Done()
		if err := ctx.Err(); err != nil {
			log.Println(err)
		}
		close(done)
	}()

	<-done
	log.Printf("finished client side")

}
