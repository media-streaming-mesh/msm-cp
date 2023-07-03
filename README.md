# MSM Control Plane

Media Streaming Mesh Control Plane

Currently implements RTSP

Goal is that control planes themselves are pluggable (we may move RTSP into a separate msm-rtsp plugin repo).

Control plane interacts with:

1. MSM-stub (to exchange control plane messages over gRPC)
2. MSM-k8s library (to write stream map entries for the network controller, 

