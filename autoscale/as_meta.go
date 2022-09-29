package autoscale

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
)

const (
	PodStateUnassigned = 0
	PodStateAssigned   = 1
	TenantStateResume  = 0
	TenantStatePause   = 0
)

type PodDesc struct {
	TenantName string
	Name       string
	IP         string
	State      int32 // 0: unassigned 1:assigned
	mu         sync.Mutex
	// pod        *v1.Pod
}

func GenMockConf() string {
	return `tmp_path = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/tmp"
	display_name = "TiFlash"
	default_profile = "default"
	users_config = "users.toml"
	path = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/db"
	capacity = "10737418240"
	mark_cache_size = 5368709120
	listen_host = "127.0.0.1"
	tcp_port = 5000
	http_port = 4500
	interserver_http_port = 5500
	
	[flash]
	tidb_status_addr = "127.0.0.1:8500"
	service_addr = "127.0.0.1:9500"
	
	[flash.flash_cluster]
	master_ttl = 60
	refresh_interval = 20
	update_rule_interval = 5
	cluster_manager_path = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/flash_cluster_manager"
	
	[flash.proxy]
	addr = "0.0.0.0:9000"
	advertise-addr = "127.0.0.1:9000"
	status-addr = "0.0.0.0:17000"
	advertise-status-addr = "127.0.0.1:17000"
	data-dir = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/db/proxy"
	config = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/conf/proxy.toml"
	log-file = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/log/proxy.log"
	log-level = "info"
	
	[logger]
	level = "debug"
	log = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/log/server.log"
	errorlog = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/log/error.log"
	
	[application]
	runAsDaemon = true
	
	[profiles]
	[profiles.default]
	max_memory_usage = 0
	max_threads = 20
	
	[raft]
	kvstore_path = "/Users/woody/Desktop/tiflash/integrated/nodes/3508/tiflash/kvstore"
	pd_addr = "127.0.0.1:6500"
	ignore_databases = "system,default"
	storage_engine="dt"
	
	[status]
	metrics_port = "127.0.0.1:17500"`
}

// TODO implement
func (c *PodDesc) AssignTenantWithMockConf(tenant string) bool {
	// simulate work for a while
	time.Sleep(time.Duration(1) * time.Second)

	return c.switchState(PodStateUnassigned, PodStateAssigned)
}

func (c *PodDesc) switchState(from int32, to int32) bool {
	return atomic.CompareAndSwapInt32(&c.State, from, to)
}

func (c *PodDesc) HandleAssignError() {
	// TODO implements
}

// TODO implement
func (c *PodDesc) UnassignTenantWithMockConf(tenant string) bool {
	// simulate work for a while
	time.Sleep(time.Duration(1) * time.Second)
	return c.switchState(PodStateAssigned, PodStateUnassigned)
}

func (c *PodDesc) HandleUnassignError() {
	// TODO implements
}

type TenantDesc struct {
	MinCntOfPod int
	MaxCntOfPod int
	pods        map[string]*PodDesc
	State       int32
	mu          sync.Mutex
}

func (c *TenantDesc) GetCntOfPods() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pods)
}

func (c *TenantDesc) SetPod(k string, v *PodDesc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pods[k] = v
}

func (c *TenantDesc) GetPod(k string) (*PodDesc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.pods[k]
	return v, ok
}

func (c *TenantDesc) RemovePod(k string, check bool) *PodDesc {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.pods[k]
	if ok {
		delete(c.pods, k)
		return v
	} else {
		return nil
	}
}

func (c *TenantDesc) GetPodNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make([]string, 0, len(c.pods))
	for k, _ := range c.pods {
		ret = append(ret, k)
	}
	return ret
}

func (c *TenantDesc) switchState(from int32, to int32) bool {
	return atomic.CompareAndSwapInt32(&c.State, from, to)
}

func (c *TenantDesc) Pause() bool {
	return c.switchState(TenantStateResume, TenantStatePause)
}

func (c *TenantDesc) Resume() bool {
	return c.switchState(TenantStatePause, TenantStateResume)
}

func (c *TenantDesc) GetState() int32 {
	return atomic.LoadInt32(&c.State)
}

const (
	DefaultMinCntOfPod        = 1
	DefaultMaxCntOfPod        = 4
	DefaultCoreOfPod          = 8
	DefaultLowerLimit         = 0.2
	DefaultHigherLimit        = 0.8
	DefaultPrewarmPoolCap     = 5
	CapacityOfStaticsAvgSigma = 6
)

func NewTenantDescDefault() *TenantDesc {
	return &TenantDesc{
		MinCntOfPod: DefaultMinCntOfPod,
		MaxCntOfPod: DefaultMaxCntOfPod,
		pods:        make(map[string]*PodDesc),
	}
}

func NewTenantDesc(minPods int, maxPods int) *TenantDesc {
	return &TenantDesc{
		MinCntOfPod: minPods,
		MaxCntOfPod: maxPods,
		pods:        make(map[string]*PodDesc),
	}
}

type AutoScaleMeta struct {
	mu sync.Mutex
	// Pod2tenant map[string]string
	tenantMap   map[string]*TenantDesc
	PodDescMap  map[string]*PodDesc
	PrewarmPods *TenantDesc
	pendingCnt  int32 // atomic
}

func NewAutoScaleMeta() *AutoScaleMeta {
	return &AutoScaleMeta{
		// Pod2tenant: make(map[string]string),
		tenantMap:   make(map[string]*TenantDesc),
		PodDescMap:  make(map[string]*PodDesc),
		PrewarmPods: NewTenantDesc(0, DefaultPrewarmPoolCap),
	}
}

func (c *AutoScaleMeta) GetTenants() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make([]string, 0, len(c.tenantMap))
	for k := range c.tenantMap {
		ret = append(ret, k)
	}
	return ret
}

// TODO remove all pod from tenant and update states
func (c *AutoScaleMeta) Pause(tenant string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.tenantMap[tenant]
	if !ok {
		return false
	}
	if v.Pause() {
		go c.removePodFromTenant(v.GetCntOfPods(), tenant)
		return true
	} else {
		return false
	}
}

// TODO add [min-cnt] pod into tenant and update states
func (c *AutoScaleMeta) Resume(tenant string, tsContainer *TimeSeriesContainer) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.tenantMap[tenant]
	if !ok {
		return false
	}
	if v.Resume() {
		go c.addPodIntoTenant(v.MinCntOfPod, tenant, tsContainer)
		return true
	} else {
		return false
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

	// c.PrewarmPods.pods[podName] = desc
	c.PrewarmPods.SetPod(podName, desc)
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

// func (c *AutoScaleMeta) getTenantLock(tenant string) *sync.Mutex {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	v, ok := c.TenantMap[tenant]
// 	if !ok {
// 		return nil
// 	} else {
// 		return &v.mu
// 	}
// }

//TODO add pod level lock!!!

// return cnt fail to add
// -1 is error
func (c *AutoScaleMeta) addPodIntoTenant(addCnt int, tenant string, tsContainer *TimeSeriesContainer) int {
	cnt := addCnt
	podsToAssign := make([]*PodDesc, 0, addCnt)
	// tMu := c.getTenantLock(tenant)
	// if tMu == nil {
	// 	return -1
	// }
	// tMu.Lock()
	// defer tMu.Unlock()
	c.mu.Lock()
	// check if tenant is valid again to prevent it has been removed
	_, ok := c.tenantMap[tenant]
	if !ok {
		c.mu.Unlock()
		// tMu.Unlock()
		return -1
	}

	podnames := c.PrewarmPods.GetPodNames()

	for _, k := range podnames {
		if cnt > 0 {
			v := c.PrewarmPods.RemovePod(k, true)
			if v != nil {
				podsToAssign = append(podsToAssign, v)

				c.tenantMap[tenant].SetPod(k, v)
				// c.tenantMap[tenant].pods[k] = v

				cnt--
			} else {
				fmt.Println("[AutoScaleMeta::addPodIntoTenant] c.PrewarmPods.RemovePod fail, return nil!")
			}
		} else {
			//enough pods, break early
			break
		}
	}
	c.mu.Unlock()
	// tMu.Unlock()
	for _, v := range podsToAssign {
		// TODO async call grpc assign api
		// TODO go func() & wg.wait()
		if !v.AssignTenantWithMockConf(tenant) {
			v.HandleAssignError()
		} else {
			// clear dirty metrics
			tsContainer.ResetMetricsOfPod(v.Name)
		}
	}
	return cnt
}

func (c *AutoScaleMeta) removePodFromTenant(removeCnt int, tenant string) int {
	cnt := removeCnt
	podsToUnassign := make([]*PodDesc, 0, removeCnt)
	// tMu := c.getTenantLock(tenant)
	// if tMu == nil {
	// 	return -1
	// }
	// tMu.Lock()
	// defer tMu.Unlock()
	c.mu.Lock()
	// check if tenant is valid again to prevent it has been removed
	tenantDesc, ok := c.tenantMap[tenant]
	if !ok {
		c.mu.Unlock()
		return -1
	}

	podnames := tenantDesc.GetPodNames()

	for _, k := range podnames {
		if cnt > 0 {
			v := tenantDesc.RemovePod(k, true)
			if v != nil {
				podsToUnassign = append(podsToUnassign, v)
				cnt--
			} else {
				fmt.Println("[AutoScaleMeta::removePodFromTenant] tenantDesc.RemovePod(k, true) fail, return nil!!!")
			}
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

			c.PrewarmPods.SetPod(v.Name, v)
			// c.PrewarmPods.pods[v.Name] = v

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
func (c *AutoScaleMeta) AddPod4Test(podName string) bool {
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

	c.PrewarmPods.SetPod(podName, podDesc)
	// c.PrewarmPods.pods[podName] = podDesc

	return true
}

func (c *AutoScaleMeta) SetupTenantWithDefaultArgs4Test(tenant string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tenantMap[tenant]
	if !ok {
		c.tenantMap[tenant] = NewTenantDescDefault()
		return true
	} else {
		return false
	}
}

func (c *AutoScaleMeta) SetupTenant(tenant string, minPods int, maxPods int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tenantMap[tenant]
	if !ok {
		c.tenantMap[tenant] = NewTenantDesc(minPods, maxPods)
		return true
	} else {
		return false
	}
}

func (c *AutoScaleMeta) UpdateTenant4Test(podName string, newTenant string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	podDesc, ok := c.PodDescMap[podName]
	if !ok {
		return false
	} else {
		// del old info
		tenantDesc, ok := c.tenantMap[podDesc.TenantName]
		if !ok {
			//TODO maintain idle pods
			// return false
		} else {

			tenantDesc.RemovePod(podName, true)
			// podMap := tenantDesc.pods
			// _, ok = podMap[podName]
			// if ok {
			// 	delete(podMap, podName)
			// }

		}
	}
	podDesc.TenantName = newTenant
	// var newPodMap map[string]*PodDesc
	newTenantDesc, ok := c.tenantMap[newTenant]
	if !ok {
		return false
		// newPodMap = make(map[string]*PodDesc)
		// c.TenantMap[newTenant] = newPodMap
	}
	newTenantDesc.SetPod(podName, podDesc)
	return true
}

func (c *AutoScaleMeta) ComputeStatisticsOfTenant(tenantName string, tsc *TimeSeriesContainer) []AvgSigma {
	c.mu.Lock()

	tenantDesc, ok := c.tenantMap[tenantName]
	if !ok {
		c.mu.Unlock()
		return nil
	} else {
		podsOfTenant := tenantDesc.GetPodNames()
		c.mu.Unlock()
		ret := make([]AvgSigma, CapacityOfStaticsAvgSigma)
		for _, podName := range podsOfTenant {
			Merge(ret, tsc.GetStatisticsOfPod(podName))
		}
		return ret
	}
}
