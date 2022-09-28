package autoscale

import (
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
)

type PodDesc struct {
	TenantName string
	Name       string
	IP         string
	// pod        *v1.Pod
}

func (c *PodDesc) AssignTenantWithMockConf(tenant string) bool {
	return false
	// TODO implement
}

func (c *PodDesc) HandleAssignError() {
	// TODO implements
}

func (c *PodDesc) UnassignTenantWithMockConf(tenant string) bool {
	return false
	// TODO implement
}

func (c *PodDesc) HandleUnassignError() {
	// TODO implements
}

type TenantDesc struct {
	MinCntOfPod int
	MaxCntOfPod int
	Pods        map[string]*PodDesc
	mu          sync.Mutex
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
	pendingCnt  int32 // atomic
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
	c.pendingCnt-- // dec pending cnt
	c.PrewarmPods.Pods[podName] = desc
}

func (c *AutoScaleMeta) handleChangeOfPodIP(pod *v1.Pod) {

}

func (c *AutoScaleMeta) UpdatePod(pod *v1.Pod) {
	name := pod.Name
	c.mu.Lock()
	defer c.mu.Unlock()
	podDesc, ok := c.PodDescMap[name]
	fmt.Printf("[updatePod] %v %v\n", name, pod.Status)
	if !ok {
		c.pendingCnt++ // inc pending cnt
		c.PodDescMap[name] = &PodDesc{Name: name, IP: pod.Status.PodIP}
		fmt.Printf("new Pod %v: %v\n", name, pod.Status.PodIP)
	} else {
		if podDesc.Name == "" {
			//TODO handle
			fmt.Printf("exception case of Pod %v\n", name)
		} else {
			if podDesc.IP == "" {
				if pod.Status.PodIP != "" {
					c.addPreWarmFromPending(name, podDesc)
					fmt.Printf("preWarm Pod %v: %v\n", name, pod.Status.PodIP)
				} else {
					fmt.Printf("preparing Pod %v\n", name)
				}

			} else {
				if pod.Status.PodIP == "" {
					//TODO handle
					c.handleAccidentalPodDeletion(pod)
					fmt.Printf("accidental Deletion of Pod %v\n", name)
				} else {
					if podDesc.IP != pod.Status.PodIP {
						c.handleChangeOfPodIP(pod)
						fmt.Printf("ipChange Pod %v: %v -> %v\n", name, podDesc.IP, pod.Status.PodIP)
					} else {
						// podDesc.pod = pod
						fmt.Printf("keep Pod %v\n", name)
					}
				}
			}
		}
	}
}

func SendGrpcReq() {

}

func (c *AutoScaleMeta) getTenantLock(tenant string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.TenantMap[tenant]
	if !ok {
		return nil
	} else {
		return &v.mu
	}
}

//TODO add pod level lock!!!

// return cnt fail to add
// -1 is error
func (c *AutoScaleMeta) addPodIntoTenant(addCnt int, tenant string) int {
	cnt := addCnt
	podsToAssign := make([]*PodDesc, 0, addCnt)
	tMu := c.getTenantLock(tenant)
	if tMu == nil {
		return -1
	}
	tMu.Lock()
	defer tMu.Unlock()
	// check if tenant is valid again to prevent it has been removed
	_, ok := c.TenantMap[tenant]
	if !ok {
		return -1
	}

	c.mu.Lock()
	for k, v := range c.PrewarmPods.Pods {
		if cnt > 0 {
			podsToAssign = append(podsToAssign, v)
			c.TenantMap[tenant].Pods[k] = v
			delete(c.PrewarmPods.Pods, k)
			cnt--
		} else {
			//enough pods, break early
			break
		}
	}
	c.mu.Unlock()
	for _, v := range podsToAssign {
		// TODO async call grpc assign api
		// TODO go func() & wg.wait()
		if !v.AssignTenantWithMockConf(tenant) {
			v.HandleAssignError()
		}
	}
	return cnt
}

func (c *AutoScaleMeta) removePodFromTenant(removeCnt int, tenant string) int {
	cnt := removeCnt
	podsToUnassign := make([]*PodDesc, 0, removeCnt)
	tMu := c.getTenantLock(tenant)
	if tMu == nil {
		return -1
	}
	tMu.Lock()
	defer tMu.Unlock()
	// check if tenant is valid again to prevent it has been removed
	tenantDesc, ok := c.TenantMap[tenant]
	if !ok {
		return -1
	}

	c.mu.Lock()
	for k, v := range tenantDesc.Pods {
		if cnt > 0 {
			podsToUnassign = append(podsToUnassign, v)
			delete(tenantDesc.Pods, k)
			// c.TenantMap[tenant].Pods[k] = v
			// delete(c.PrewarmPods.Pods, k)
			cnt--
		} else {
			//enough pods, break early
			break
		}
	}
	c.mu.Unlock()
	for _, v := range podsToUnassign {
		// TODO async call grpc unassign api
		// TODO go func() & wg.wait()
		if !v.UnassignTenantWithMockConf(tenant) {
			v.HandleUnassignError()
		} else {
			c.mu.Lock()
			c.PrewarmPods.Pods[v.Name] = v
			c.mu.Unlock()
		}
	}
	return cnt
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

// for test
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
