package autoscale

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

const (
	PodStateUnassigned      = 0
	PodStateAssigned        = 1
	PodStateInit            = 2
	TenantStateResume       = 0
	TenantStatePause        = 1
	CmRnPodStateUnassigned  = 0
	CmRnPodStateUnassigning = 1
	CmRnPodStateAssigning   = 2
	CmRnPodStateAssigned    = 3
	CmRnPodStateUnknown     = -1
)

type PodDesc struct {
	TenantName string
	Name       string
	IP         string
	State      int32      // 0: unassigned 1:assigned
	mu         sync.Mutex // TODO use it
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
	Name        string
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

func (c *TenantDesc) SyncStatePause() bool {
	return c.switchState(TenantStateResume, TenantStatePause)
}

func (c *TenantDesc) SyncStateResume() bool {
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
	DefaultUpperLimit         = 0.8
	DefaultPrewarmPoolCap     = 5
	CapacityOfStaticsAvgSigma = 6
)

func NewTenantDescDefault(name string) *TenantDesc {
	return &TenantDesc{
		Name:        name,
		MinCntOfPod: DefaultMinCntOfPod,
		MaxCntOfPod: DefaultMaxCntOfPod,
		pods:        make(map[string]*PodDesc),
	}
}

func NewTenantDesc(name string, minPods int, maxPods int) *TenantDesc {
	return &TenantDesc{
		Name:        name,
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

	k8sCli    *kubernetes.Clientset
	configMap *v1.ConfigMap //TODO expire entry  of removed pod
	cmMutex   sync.Mutex
}

func NewAutoScaleMeta(config *restclient.Config) *AutoScaleMeta {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	ret := &AutoScaleMeta{
		// Pod2tenant: make(map[string]string),
		tenantMap:   make(map[string]*TenantDesc),
		PodDescMap:  make(map[string]*PodDesc),
		PrewarmPods: NewTenantDesc("$prewarm", 0, DefaultPrewarmPoolCap),
		k8sCli:      client,
	}
	ret.loadTenants()
	ret.initConfigMap()
	return ret
}

func (c *AutoScaleMeta) loadTenants() {
	c.SetupTenant("t1", 1, 4)
	fmt.Printf("loadTenant, SetupTenant(t1, 1, 4)\n")
	//TODO load tenants from config of control panel
}

func (c *AutoScaleMeta) initConfigMap() {
	configMapName := "readnode-pod-state"
	var err error
	c.configMap, err = c.k8sCli.CoreV1().ConfigMaps("tiflash-autoscale").Get(context.TODO(), configMapName, metav1.GetOptions{})
	if err != nil {
		// panic(err.Error())
		c.configMap = &v1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				Kind:       "ConfigMap",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: configMapName,
			},
			Data: map[string]string{},
		}
		// for {
		// get pods in all the namespaces by omitting namespace
		// Or specify namespace to get pods in particular namespace
		c.configMap, err = c.k8sCli.CoreV1().ConfigMaps("tiflash-autoscale").Create(context.TODO(), c.configMap, metav1.CreateOptions{})
		if err != nil {
			panic(err.Error())
		}
	}
	fmt.Printf("loadConfigMap %v\n", c.configMap.String())
}

func (c *AutoScaleMeta) RecoverStatesOfPods4Test() {
	// c.mu.Lock()
	// defer c.mu.Unlock()
	// for podname, poddesc := range c.PodDescMap {
	// 	poddesc.switchState()
	// }
}

func (c *AutoScaleMeta) GetTenants() []*TenantDesc {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make([]*TenantDesc, 0, len(c.tenantMap))
	for _, v := range c.tenantMap {
		ret = append(ret, v)
	}
	return ret
}

func (c *AutoScaleMeta) GetTenantNames() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	ret := make([]string, 0, len(c.tenantMap))
	for _, v := range c.tenantMap {
		ret = append(ret, v.Name)
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
	if v.SyncStatePause() {
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
	if v.SyncStateResume() {
		// TODO ensure there is no pods now
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

func ConfigMapPodStateStr(state int, optTenant string) string {
	var value string
	switch state {
	case CmRnPodStateUnassigned:
		value = "unassigned"
	case CmRnPodStateUnassigning:
		value = "unassigning"
	case CmRnPodStateAssigned:
		value = "assigned|" + optTenant
	case CmRnPodStateAssigning:
		value = "assigning"
	}
	return value
}

func ConfigMapPodState(str string) (int, string) {
	switch str {
	case "unassigned":
		return CmRnPodStateUnassigned, ""
	case "unassigning":
		return CmRnPodStateUnassigning, ""
	case "assigning":
		return CmRnPodStateAssigning, ""
	default:
		if !strings.HasPrefix(str, "assigned|") {
			return -1, ""
		}
		i := strings.Index(str, "|")
		tenant := ""
		if i > -1 {
			tenant = str[i+1:]
		}
		return CmRnPodStateAssigned, tenant
	}

}

func (c *AutoScaleMeta) handleK8sConfigMapsApiError(err error, caller string) {
	configMapName := "readnode-pod-state"
	errStr := err.Error()
	fmt.Printf("[error][%v]K8sConfigMapsApiError, err: %+v\n", caller, errStr)
	if strings.Contains(errStr, "please apply your changes to the latest version") {
		retConfigMap, err := c.k8sCli.CoreV1().ConfigMaps("tiflash-autoscale").Get(context.TODO(), configMapName, metav1.GetOptions{})
		if err != nil {
			fmt.Printf("[error][%v]K8sConfigMapsApiError, failed to get latest version of configmap, err: %+v\n", caller, err.Error())
		} else {
			c.configMap = retConfigMap
		}
	}
}

// TODO mutex protect by COnfigMapMutex
//
//	func (c *AutoScaleMeta) configMapStateWarming(podName string) {
//		c.configMap.Data[podName] = "unassigned"
//	}
func (c *AutoScaleMeta) setConfigMapState(podName string, state int, optTenant string) error {
	c.cmMutex.Lock()
	defer c.cmMutex.Unlock()
	var value string
	switch state {
	case CmRnPodStateUnassigned:
		value = "unassigned"
	case CmRnPodStateUnassigning:
		value = "unassigning"
	case CmRnPodStateAssigned:
		value = "assigned|" + optTenant
	case CmRnPodStateAssigning:
		value = "assigning"
	}
	// TODO  c.configMap.DeepCopy()
	// TODO err handlling
	if c.configMap.Data == nil {
		c.configMap.Data = make(map[string]string)
	}
	c.configMap.Data[podName] = value
	retConfigMap, err := c.k8sCli.CoreV1().ConfigMaps("tiflash-autoscale").Update(context.TODO(), c.configMap, metav1.UpdateOptions{})
	if err != nil {
		c.handleK8sConfigMapsApiError(err, "AutoScaleMeta::setConfigMapState")
		return err
	}
	c.configMap = retConfigMap
	return nil
}

func (c *AutoScaleMeta) setConfigMapStateBatch(kvMap map[string]string) error {
	c.cmMutex.Lock()
	defer c.cmMutex.Unlock()
	// TODO  c.configMap.DeepCopy()
	// TODO err handlling
	if c.configMap.Data == nil {
		c.configMap.Data = make(map[string]string)
	}
	for k, v := range kvMap {
		c.configMap.Data[k] = v
	}

	retConfigMap, err := c.k8sCli.CoreV1().ConfigMaps("tiflash-autoscale").Update(context.TODO(), c.configMap, metav1.UpdateOptions{})
	if err != nil {
		c.handleK8sConfigMapsApiError(err, "AutoScaleMeta::setConfigMapState")
		return err
	}
	fmt.Printf("[AutoScaleMeta]current configmap: %+v\n", retConfigMap.Data)
	c.configMap = retConfigMap
	return nil
}

func (c *AutoScaleMeta) addPreWarmFromPending(podName string, desc *PodDesc) {
	fmt.Printf("[AutoScaleMeta]addPreWarmFromPending %v\n", podName)
	c.pendingCnt-- // dec pending cnt

	// c.PrewarmPods.pods[podName] = desc
	c.PrewarmPods.SetPod(podName, desc)
	c.setConfigMapState(podName, CmRnPodStateUnassigned, "")
}

func (c *AutoScaleMeta) handleChangeOfPodIP(pod *v1.Pod) {

}

func (c *AutoScaleMeta) GetRnPodStateAndTenantFromCM(podname string) (int, string) {
	c.cmMutex.Lock()
	defer c.cmMutex.Unlock()
	stateStr, ok := c.configMap.Data[podname]
	if !ok {
		return CmRnPodStateUnknown, ""
	} else {
		return ConfigMapPodState(stateStr)
	}
}

func (c *AutoScaleMeta) UpdatePod(pod *v1.Pod) {
	name := pod.Name
	c.mu.Lock()
	defer c.mu.Unlock()
	podDesc, ok := c.PodDescMap[name]
	fmt.Printf("[updatePod] %v cur_ip:%v\n", name, pod.Status.PodIP)
	if !ok { // new pod
		c.pendingCnt++ // inc pending cnt
		state := PodStateInit
		tenantName := ""
		cmRnPodState := CmRnPodStateUnknown
		if pod.Status.PodIP != "" {
			state = PodStateUnassigned // TODO use api/etcd to get read state(assign or unassign, which tenant ...) in future
			cmRnPodState, tenantName = c.GetRnPodStateAndTenantFromCM(name)
			// TODO handle ssigning case ,  merge podstate/CmRnPodState together
			if cmRnPodState != CmRnPodStateUnknown && cmRnPodState == CmRnPodStateAssigned {
				state = PodStateAssigned
			}
		}
		podDesc = &PodDesc{Name: name, IP: pod.Status.PodIP, State: int32(state)}
		c.PodDescMap[name] = podDesc
		if state == PodStateUnassigned {
			c.addPreWarmFromPending(name, podDesc)
			fmt.Printf("[UpdatePod]addPreWarmFromPending %v: %v\n", name, pod.Status.PodIP)
		} else if state == PodStateAssigned {
			c.updateLocalMetaPodOfTenant(name, podDesc, tenantName)
			fmt.Printf("[UpdatePod]updateLocalMetaPodOfTenant %v: %v tenant: %v\n", name, pod.Status.PodIP, tenantName)
		}

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

// TODO refine lock logic to prevent race
// TODO make it non-blocking between tenants
func (c *AutoScaleMeta) ResizePodsOfTenant(from int, target int, tenant string, tsContainer *TimeSeriesContainer) {
	fmt.Printf("[AutoScaleMeta]ResizePodsOfTenant from %v to %v , tenant:%v\n", from, target, tenant)
	// TODO assert and validate "from" equal to current cntOfPod
	if target > from {
		c.addPodIntoTenant(target-from, tenant, tsContainer)
	} else if target < from {
		c.removePodFromTenant(from-target, tenant)
	}
}

func (c *AutoScaleMeta) updateLocalMetaPodOfTenant(podName string, podDesc *PodDesc, tenant string) {
	fmt.Printf("[AutoScaleMeta]updateLocalMetaPodOfTenant pod:%v tenant:%v\n", podName, tenant)
	// c.mu.Lock()
	// defer c.mu.Unlock()
	tDesc, ok := c.tenantMap[tenant]
	if ok {
		tDesc.SetPod(podName, podDesc)
	} else {
		fmt.Printf("wild pod found! pod:%v , tenant: %v\n", podName, tenant)
	}
}

//TODO add pod level lock!!!

// return cnt fail to add
// -1 is error
func (c *AutoScaleMeta) addPodIntoTenant(addCnt int, tenant string, tsContainer *TimeSeriesContainer) int {
	fmt.Printf("[AutoScaleMeta::addPodIntoTenant] %v %v \n", addCnt, tenant)
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
				// TODO !!! DO we need setPod after assigned successfully? since assign api may fail
				c.tenantMap[tenant].SetPod(k, v)
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
	statesDeltaMap := make(map[string]string)
	for _, v := range podsToAssign {
		// TODO async call grpc assign api
		// TODO go func() & wg.wait()
		if !v.AssignTenantWithMockConf(tenant) {
			v.HandleAssignError()
		} else {
			statesDeltaMap[v.Name] = ConfigMapPodStateStr(CmRnPodStateAssigned, tenant)
			// clear dirty metrics
			tsContainer.ResetMetricsOfPod(v.Name)
		}
	}
	c.setConfigMapStateBatch(statesDeltaMap)
	return cnt
}

func (c *AutoScaleMeta) removePodFromTenant(removeCnt int, tenant string) int {
	fmt.Printf("[AutoScaleMeta::removePodFromTenant] %v %v \n", removeCnt, tenant)
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
	statesDeltaMap := make(map[string]string)
	for _, v := range podsToUnassign {
		// TODO async call grpc unassign api
		// TODO go func() & wg.wait()
		if !v.UnassignTenantWithMockConf(tenant) {
			v.HandleUnassignError()
		} else {
			c.mu.Lock()
			statesDeltaMap[v.Name] = ConfigMapPodStateStr(CmRnPodStateUnassigned, "")
			c.PrewarmPods.SetPod(v.Name, v)
			// c.PrewarmPods.pods[v.Name] = v

			c.mu.Unlock()
		}
	}
	c.setConfigMapStateBatch(statesDeltaMap)
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
		c.tenantMap[tenant] = NewTenantDescDefault(tenant)
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
		c.tenantMap[tenant] = NewTenantDesc(tenant, minPods, maxPods)
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

func MockComputeStatisticsOfTenant(coresOfPod int, cntOfPods int, maxCntOfPods int) float64 {
	ts := time.Now().Unix() / 2
	// tsInMins := ts / 60
	return math.Min((math.Sin(float64(ts)/10.0)+1)/2*float64(coresOfPod)*float64(maxCntOfPods)/float64(cntOfPods), 8)
}
