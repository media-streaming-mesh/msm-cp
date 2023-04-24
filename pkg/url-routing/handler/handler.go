package handler

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"strconv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type UrlHandler struct {
	clientset        kubernetes.Interface
	currentNamespace string
}

func (uh *UrlHandler) InitializeUrlHandler() {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal(err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("error creating out of cluster config: %v", err)
	}

	uh.clientset = clientset
	uh.verifyK8sApi()

	uh.log("connected to url-routing-server")

	// Get the current namespace of the pod
	currentNamespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		log.Fatalf("unable to read current namespace")
	}

	uh.log("current namespace is %v", string(currentNamespace))
	uh.currentNamespace = string(currentNamespace)
}

func (uh *UrlHandler) verifyK8sApi() {
	_, err := uh.clientset.CoreV1().Nodes().List(context.TODO(), v1.ListOptions{})
	if err != nil {
		log.Fatalf("could not connect to url-routing-server: %v", err)
	}
}

func (uh *UrlHandler) log(format string, args ...interface{}) {
	// keep remote address outside format, since it can contain %
	log.Println("[K8S API Client] " + fmt.Sprintf(format, args...))
}

func (uh *UrlHandler) GetInternalURLs(Url string) []string {
	uh.log("url provided: %s", Url)
	u, err := url.Parse(Url)
	if err != nil {
		uh.log("could not parse url: %v", err)
		return []string{}
	}

	// lookup IP and port
	endpoints := uh.resolveEndpoints(u.Hostname(), u.Port(), u.Path)
	urls := make([]string, len(endpoints))

	// add the original path to the resolved internal endpoint
	for i, endpoint := range endpoints {
		cpy := u
		cpy.Host = endpoint
		urls[i] = cpy.String()
	}
	return urls
}

func (uh *UrlHandler) resolveEndpoints(hostname string, port string, path string) []string {
	var (
		serviceName string
		addresses   []string
	)

	uh.log("url components %s, %s, %s", hostname, port, path)

	if hostname == "localhost" {
		if len(path) > 0 {
			uh.log("IP is localhost - path is %s", path)
			serviceName = path[1:]
		}
	} else {
		// look up hostname in DNS if needed
		addresses = uh.resolveHost(hostname)

		// if DNS resolved to set of addrs, return addrs
		if len(addresses) > 1 {
			uh.log("DNS returned %d results", len(addresses))
			endpoints := make([]string, len(addresses))
			for i := range addresses {
				endpoints[i] = addresses[i] + ":" + port + path
			}
			return endpoints
		}

		// now look for the URL suffix and match to a K8s service name
		if len(path) > 0 {
			serviceName = path[1:]
		}
	}

	// else if single ip is a clusterIP, return endpoints
	if serviceName == "" {
		var err error
		serviceName, err = uh.getServiceName(addresses[0])

		// if ip is not clusterIP, return ip
		if err != nil {
			uh.log("unable to look up cluster IP, returning IP given (%s)", addresses[0])
			return []string{addresses[0] + ":" + port}
		}

		uh.log("ClusterIP's service name is %s", serviceName)
	}

	// search for endpoints that belong to this service
	uh.log("Path or clusterIP was resolved to the service: %s", serviceName)
	return uh.getEndpoints(serviceName)
}

// get all endpoints for a named service
func (uh *UrlHandler) getEndpoints(serviceName string) []string {
	ends, err := uh.clientset.CoreV1().Endpoints(uh.currentNamespace).Get(context.TODO(), serviceName, v1.GetOptions{})
	if err != nil {
		uh.log("failed to get service endpoints")
		return []string{}
	}

	endpoints := make([]string, 0)
	for _, end := range ends.Subsets {
		addrs := end.Addresses
		ports := end.Ports

		for _, addr := range addrs {
			for _, port := range ports {
				uh.log("service endpoint: %s:%d", addr.IP, port.Port)
				endpoint := addr.IP + ":" + strconv.FormatUint(uint64(port.Port), 10)
				endpoints = append(endpoints, endpoint)
			}
		}
	}

	return endpoints
}

// look for a service with clusterIP matching a given IP
func (uh *UrlHandler) getServiceName(clusterIP string) (string, error) {
	services, err := uh.clientset.CoreV1().Services(uh.currentNamespace).List(context.TODO(),
		v1.ListOptions{})
	if err != nil {
		uh.log("unable to get list of services")
		return "", nil
	}

	for _, service := range services.Items {
		if service.Spec.ClusterIP == clusterIP {
			uh.log("found service name %s for clusterIP %s", service.Name, clusterIP)
			return service.Name, nil
		}
	}

	return "", fmt.Errorf("service name could not be resolved")
}

// if given hostname is an IP return it, else perform DNS lookup
// DNS may return multiple IPs (uh.g. headless services)
func (uh *UrlHandler) resolveHost(host string) []string {
	var ip net.IP

	uh.log("host = %s", host)

	ip = net.ParseIP(host) // returns nil if not ip

	// if host is not an ip, perform dns lookup
	if ip == nil {
		uh.log("looking up in DNS")
		addrs, err := net.LookupHost(host)
		if err != nil {
			uh.log("could not resolve resource to ips: %s, returning given resource: %s", err.Error(), host)
			return []string{host}
		}
		uh.log("resolution: %s", addrs[0])
		return addrs
	}
	// dns not needed, already an ip
	uh.log("resolution: %v", ip)
	return []string{ip.String()}
}
