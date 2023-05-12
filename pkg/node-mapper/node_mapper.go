package node_mapper

import (
	"context"
	"fmt"
	"github.com/media-streaming-mesh/msm-cp/pkg/config"
	"github.com/media-streaming-mesh/msm-cp/pkg/model"
	"log"
	"net"
	"sync"

	"github.com/sirupsen/logrus"

	v12 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var NodeMap *sync.Map

type NodeMapper struct {
	clientset kubernetes.Interface
	logger    *logrus.Logger
}

func InitializeNodeMapper(cfg *config.Cfg) *NodeMapper {
	NodeMap = new(sync.Map)
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("error creating out of cluster config: %v", err)
	}

	return &NodeMapper{
		clientset: clientset,
		logger:    cfg.Logger,
	}
}

func (mapper *NodeMapper) log(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	mapper.logger.Infof("[Node Mapper] " + fmt.Sprintf(format, args...))
}

func (mapper *NodeMapper) logError(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	mapper.logger.Errorf("[Node Mapper] " + fmt.Sprintf(format, args...))
}

func MapNode(ip string) (string, error) {
	var nodeIP string
	NodeMap.Range(func(key, value interface{}) bool {
		if key == ip {
			nodeIP = value.(string)
		} else {
			_, nodeIPNet, error := net.ParseCIDR(key.(string))
			if error == nil {
				if nodeIPNet.Contains(net.ParseIP(ip)) {
					nodeIP = value.(string)
				}
			}
		}

		return true
	})

	if nodeIP != "" {
		return nodeIP, nil
	}
	return "", fmt.Errorf("Can't get node ip")
}

func IsOnSameNode(ip string, ip2 string) bool {
	nodeIp, err := MapNode(ip)
	node2Ip, err2 := MapNode(ip2)

	if err != nil || err2 != nil {
		return false
	}
	if nodeIp != node2Ip {
		return false
	}
	return true
}

func (mapper *NodeMapper) WatchNode(nodeChan chan<- model.Node) {
	watcher, err := mapper.clientset.CoreV1().Nodes().Watch(context.TODO(), v1.ListOptions{})
	if err != nil {
		mapper.logError("watcher err %v", err)
	}
	mapper.log("Start watch node")
	ch := watcher.ResultChan()
	for event := range ch {
		node, ok := event.Object.(*v12.Node)
		if ok {
			switch event.Type {
			case watch.Added:
				mapper.addNode(node, nodeChan)
			case watch.Deleted:
				mapper.deleteNode(node, nodeChan)
			}
		}
	}
}

func (mapper *NodeMapper) addNode(node *v12.Node, nodeChan chan<- model.Node) {
	mapper.log("Added node %v", node.Name)
	mapper.log("Node PodCIDR %v", node.Spec.PodCIDR)
	mapper.log("Node Calico IPv4Address %v", node.Annotations["projectcalico.org/IPv4Address"])
	mapper.log("Node Calico IPv4IPIPTunnelAddr %v", node.Annotations["projectcalico.org/IPv4IPIPTunnelAddr"])

	key := mapper.getKey(node)

	// Save ip to hashmap
	for _, address := range node.Status.Addresses {
		if address.Type == v12.NodeInternalIP {
			NodeMap.Store(node.Name, address.Address)

			// TODO remove these when use stubIp
			NodeMap.Store(key, address.Address)
			NodeMap.Store(address.Address+"/32", address.Address) // store nodes's own IP in the map

			mapper.log("Store node Internal IP %v with key %v", address.Address, key)
			mapper.log("Store node Internal IP %v with key %v", address.Address, address.Address+"/32")
			mapper.log("Store node Internal IP %v with key %v", address.Address, node.Name)

			//Send node to chanel
			nodeChan <- model.Node{
				node.Name,
				address.Address,
				model.AddNode,
			}
			break
		}
	}
}

func (mapper *NodeMapper) deleteNode(node *v12.Node, nodeChan chan<- model.Node) {
	mapper.log("Delete node %v", node.Name)

	key := mapper.getKey(node)

	for _, address := range node.Status.Addresses {
		if address.Type == v12.NodeInternalIP {
			NodeMap.Delete(node.Name)
			NodeMap.Delete(key)
			NodeMap.Delete(address.Address + "/32")

			nodeChan <- model.Node{
				node.Name,
				address.Address,
				model.DeleteNode,
			}
			break
		}
	}
}

func (mapper *NodeMapper) getKey(node *v12.Node) string {
	// Key is CIDR
	var key string
	// calicoIPv4 := node.Annotations["projectcalico.org/IPv4Address"]
	calicoIPv4PIPTunnel := node.Annotations["projectcalico.org/IPv4IPIPTunnelAddr"]

	if calicoIPv4PIPTunnel == "" {
		key = node.Spec.PodCIDR
	} else {
		// n := strings.LastIndex(calicoIPv4, "/")
		// subnet := calicoIPv4[n:]
		key = calicoIPv4PIPTunnel + "/26"
	}
	return key
}
