package netpol

import (
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	api "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	utilsnet "k8s.io/utils/net"
)

const (
	PodCompleted api.PodPhase = "Completed"
)

// isPodUpdateNetPolRelevant checks the attributes that we care about for building NetworkPolicies on the host and if it
// finds a relevant change, it returns true otherwise it returns false. The things we care about for NetworkPolicies:
// 1) Is the phase of the pod changing? (matters for catching completed, succeeded, or failed jobs)
// 2) Is the pod IP changing? (changes how the network policy is applied to the host)
// 3) Is the pod's host IP changing? (should be caught in the above, with the CNI kube-router runs with but we check this as well for sanity)
// 4) Is a pod's label changing? (potentially changes which NetworkPolicies select this pod)
func isPodUpdateNetPolRelevant(oldPod, newPod *api.Pod) bool {
	return newPod.Status.Phase != oldPod.Status.Phase ||
		newPod.Status.PodIP != oldPod.Status.PodIP ||
		!reflect.DeepEqual(newPod.Status.PodIPs, oldPod.Status.PodIPs) ||
		newPod.Status.HostIP != oldPod.Status.HostIP ||
		!reflect.DeepEqual(newPod.Labels, oldPod.Labels)
}

func isNetPolActionable(pod *api.Pod) bool {
	return !isFinished(pod) && pod.Status.PodIP != "" && !pod.Spec.HostNetwork
}

func isFinished(pod *api.Pod) bool {
	switch pod.Status.Phase {
	case api.PodFailed, api.PodSucceeded, PodCompleted:
		return true
	}
	return false
}

func validateNodePortRange(nodePortOption string) (string, error) {
	nodePortValidator := regexp.MustCompile(`^([0-9]+)[:-]([0-9]+)$`)
	if matched := nodePortValidator.MatchString(nodePortOption); !matched {
		return "", fmt.Errorf("failed to parse node port range given: '%s' please see specification in help text", nodePortOption)
	}
	matches := nodePortValidator.FindStringSubmatch(nodePortOption)
	if len(matches) != 3 {
		return "", fmt.Errorf("could not parse port number from range given: '%s'", nodePortOption)
	}
	port1, err := strconv.ParseUint(matches[1], 10, 16)
	if err != nil {
		return "", fmt.Errorf("could not parse first port number from range given: '%s'", nodePortOption)
	}
	port2, err := strconv.ParseUint(matches[2], 10, 16)
	if err != nil {
		return "", fmt.Errorf("could not parse second port number from range given: '%s'", nodePortOption)
	}
	if port1 >= port2 {
		return "", fmt.Errorf("port 1 is greater than or equal to port 2 in range given: '%s'", nodePortOption)
	}
	return fmt.Sprintf("%d:%d", port1, port2), nil
}

func getIPsFromPods(pods []podInfo, family api.IPFamily) []string {
	var ips []string
	for _, pod := range pods {
		switch family {
		case api.IPv4Protocol:
			ip, err := getPodIPv4Address(pod)
			if err != nil {
				klog.Warningf("Could not get IPv4 addresses of all pods: %v", err)
				continue
			}
			ips = append(ips, ip)
		case api.IPv6Protocol:
			ip, err := getPodIPv6Address(pod)
			if err != nil {
				klog.Warningf("Could not get IPv6 addresses of all pods: %v", err)
				continue
			}
			ips = append(ips, ip)
		}
	}
	return ips
}

func getPodIPv6Address(pod podInfo) (string, error) {
	for _, ip := range pod.ips {
		if utilsnet.IsIPv6String(ip.IP) {
			return ip.IP, nil
		}
	}
	return "", fmt.Errorf("pod %s has no IPv6Address", pod.name)
}

func getPodIPv4Address(pod podInfo) (string, error) {
	for _, ip := range pod.ips {
		if utilsnet.IsIPv4String(ip.IP) {
			return ip.IP, nil
		}
	}
	return "", fmt.Errorf("pod %s has no IPv4Address", pod.name)
}
