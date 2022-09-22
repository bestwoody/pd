package autoscale

import (
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
)

type PodDesc struct {
	TenantName string
	pod        *v1.Pod
}

type TenantDesc struct {
	MinCntOfPod int
	MaxCntOfPod int
	Pods        map[string]*PodDesc
}

const (
	DefaultMinCntOfPod    = 1
	DefaultMaxCntOfPod    = 4
	DefaultCoreOfPod      = 8
	DefaultLowerLimit     = 0.2
	DefaultHigherLimit    = 0.8
	DefaultPrewarmPoolCap = 5
)

func NewTenantDescDefault() *TenantDesc {
	return &TenantDesc{
		MinCntOfPod: DefaultMinCntOfPod,
		MaxCntOfPod: DefaultMaxCntOfPod,
		Pods:        make(map[string]*PodDesc),
	}
}

func NewTenantDesc(minPods int, maxPods int) *TenantDesc {
	return &TenantDesc{
		MinCntOfPod: minPods,
		MaxCntOfPod: maxPods,
		Pods:        make(map[string]*PodDesc),
	}
}

type AutoScaleMeta struct {
	mu sync.Mutex
	// Pod2tenant map[string]string
	TenantMap   map[string]*TenantDesc
	PodDescMap  map[string]*PodDesc
	PrewarmPods *TenantDesc
}

func NewAutoScaleMeta() *AutoScaleMeta {
	return &AutoScaleMeta{
		// Pod2tenant: make(map[string]string),
		TenantMap:   make(map[string]*TenantDesc),
		PodDescMap:  make(map[string]*PodDesc),
		PrewarmPods: NewTenantDesc(0, DefaultPrewarmPoolCap),
	}
}

func (cur *AutoScaleMeta) CreateOrGetPodDesc(podName string, createOrGet bool) *PodDesc {
	val, ok := cur.PodDescMap[podName]
	if !ok {
		// if !createIfNotExist {
		// 	return nil
		// }
		if !createOrGet { // should create but get is true
			return nil
		}
		ret := &PodDesc{}
		cur.PodDescMap[podName] = ret
		return ret
	} else {
		if createOrGet { // should get but create is true
			return nil
		}
		return val
	}
}

func (c *AutoScaleMeta) AddPodDetail(podName string, pod *v1.Pod) {
	// TODO implements
}

func (c *AutoScaleMeta) addPreWarmFromPending(podName string, desc *PodDesc) {
	c.PrewarmPods.Pods[podName] = desc
}

func (c *AutoScaleMeta) handleChangeOfPodIP(pod *v1.Pod) {

}

func (c *AutoScaleMeta) UpdatePod(pod *v1.Pod) {
	name := pod.Name
	c.mu.Lock()
	defer c.mu.Unlock()
	podDesc, ok := c.PodDescMap[name]
	if !ok {
		c.PodDescMap[name] = &PodDesc{pod: pod}
		fmt.Printf("new Pod %v\n", name)
	} else {
		if podDesc.pod == nil {
			//TODO handle
			fmt.Printf("exception case of Pod %v\n", name)
		} else {
			if podDesc.pod.Status.PodIP == "" {
				if pod.Status.PodIP != "" {
					c.addPreWarmFromPending(name, podDesc)
					fmt.Printf("preWarm Pod %v\n", name)
				} else {
					fmt.Printf("preparing Pod %v\n", name)
				}

			} else {
				if podDesc.pod.Status.PodIP != pod.Status.PodIP {
					c.handleChangeOfPodIP(pod)
					fmt.Printf("ipChange Pod %v\n", name)
				} else {
					podDesc.pod = pod
					fmt.Printf("keep Pod %v\n", name)
				}
			}
		}
	}
}

func (c *AutoScaleMeta) removePodFromTenant(pod *v1.Pod) {
	/// TODO implemants
}

/// TODO  since we del pod on our own, we should think of corner case that accidental pod deletion by k8s

func (c *AutoScaleMeta) handleAccidentalPodDeletion(pod *v1.Pod) {

}

func (c *AutoScaleMeta) HandleK8sDelPodEvent(pod *v1.Pod) bool {
	name := pod.Name
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.PodDescMap[name]
	if !ok {
		return true
	} else {
		c.handleAccidentalPodDeletion(pod)
		return false
	}

	// TODO implements
}

func (c *AutoScaleMeta) AddPod(podName string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.PodDescMap[podName]
	if ok {
		return false
	}
	// podDesc := c.CreateOrGetPodDesc(podName, true)
	// if podDesc == nil {
	// return false
	// }
	podDesc := &PodDesc{}
	c.PodDescMap[podName] = podDesc
	c.PrewarmPods.Pods[podName] = podDesc
	return true
}

func (c *AutoScaleMeta) SetupTenantWithDefaultArgs(tenant string) bool {
	_, ok := c.TenantMap[tenant]
	if !ok {
		c.TenantMap[tenant] = NewTenantDescDefault()
		return true
	} else {
		return false
	}
}

func (c *AutoScaleMeta) SetupTenant(tenant string, minPods int, maxPods int) bool {
	_, ok := c.TenantMap[tenant]
	if !ok {
		c.TenantMap[tenant] = NewTenantDesc(minPods, maxPods)
		return true
	} else {
		return false
	}
}

func (c *AutoScaleMeta) UpdateTenant(podName string, newTenant string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	podDesc, ok := c.PodDescMap[podName]
	if !ok {
		return false
	} else {
		// del old info
		tenantDesc, ok := c.TenantMap[podDesc.TenantName]
		if !ok {
			//TODO maintain idle pods
			// return false
		} else {
			podMap := tenantDesc.Pods
			_, ok = podMap[podName]
			if ok {
				delete(podMap, podName)
			}
		}
	}
	podDesc.TenantName = newTenant
	// var newPodMap map[string]*PodDesc
	newTenantDesc, ok := c.TenantMap[newTenant]
	if !ok {
		return false
		// newPodMap = make(map[string]*PodDesc)
		// c.TenantMap[newTenant] = newPodMap
	}
	newTenantDesc.Pods[podName] = podDesc
	return true
}

func (c *AutoScaleMeta) ComputeStatisticsOfTenant(tenantName string, tsc *TimeSeriesContainer) []AvgSigma {
	c.mu.Lock()
	defer c.mu.Unlock()
	tenantDesc, ok := c.TenantMap[tenantName]
	if !ok {
		return nil
	} else {
		ret := make([]AvgSigma, 6)
		for podName := range tenantDesc.Pods {
			Merge(ret, tsc.SeriesMap[podName].Statistics)
		}
		return ret
	}
}
