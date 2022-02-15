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
	"os"

	pb "github.com/media-streaming-mesh/msm-cp/api/v1alpha1/msm_cp"
	"google.golang.org/grpc"
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
	ctx := context.Background()

	req := &pb.Endpoints{
		Source: "192.168.1.1",
		Dest:   "10.0.0.1",
	}

	res, err := client.ClientConnect(ctx, req)
	if err != nil {
		fmt.Println("Houston, we've got a problem!")
	}
	fmt.Printf("Response: %+v\n", res)

	cReq := &pb.Request{
		Request: "SETUP rtsp://localhost:8554/foo",
	}
	res2, err := client.ClientRequest(ctx, cReq)
	if err != nil {
		fmt.Println("Houston, we've got a problem!")
	}
	fmt.Printf("Response: %+v\n", res2)

	fmt.Printf("GrpcClient done...\n")

}
