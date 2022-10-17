package autoscale

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise-api/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

func outsideConfig() (*restclient.Config, error) {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	return clientcmd.BuildConfigFromFlags("", *kubeconfig)
}

func getK8sConfig() (*restclient.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return outsideConfig()
	} else {
		return config, err
	}
}

// TODO mutex protection
type ClusterManager struct {
	Namespace     string
	CloneSetName  string
	AutoScaleMeta *AutoScaleMeta
	K8sCli        *kubernetes.Clientset
	MetricsCli    *metricsv.Clientset
	Cli           *kruiseclientset.Clientset
	CloneSet      *v1alpha1.CloneSet
	wg            sync.WaitGroup
	shutdown      int32 // atomic
	watchMu       sync.Mutex
	watcher       watch.Interface

	tsContainer *TimeSeriesContainer
	lstTsMap    map[string]int64
}

// TODO expire of removed Pod in tsContainer,lstTsMap

func (c *ClusterManager) collectMetrics() {
	defer c.wg.Done()
	as_meta := c.AutoScaleMeta
	lstTsMap := c.lstTsMap
	tsContainer := c.tsContainer
	hasNew := false
	for {
		time.Sleep(200 * time.Millisecond)
		if atomic.LoadInt32(&c.shutdown) != 0 {
			return
		}
		labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"app": c.CloneSetName}}
		// st := time.Now().UnixNano()
		podMetricsList, err := c.MetricsCli.MetricsV1beta1().PodMetricses(c.Namespace).List(
			context.TODO(), metav1.ListOptions{LabelSelector: labels.Set(labelSelector.MatchLabels).String()})
		if err != nil {
			panic(err.Error())
		}
		mint := int64(math.MaxInt64)
		maxt := int64(0)
		// et := time.Now().UnixNano()
		// log.Printf("[ClusterManager]collectMetrics loop, api cost: %v ns\n", et-st)
		// pendLogStr := ""
		for _, pod := range podMetricsList.Items {
			lstTs, ok := lstTsMap[pod.Name]
			if !ok || pod.Timestamp.Unix() > lstTs {
				tsContainer.Insert(pod.Name, pod.Timestamp.Unix(),
					[]float64{
						pod.Containers[0].Usage.Cpu().AsApproximateFloat64(),
						pod.Containers[0].Usage.Memory().AsApproximateFloat64(),
					})
				lstTsMap[pod.Name] = pod.Timestamp.Unix()

				// if pod.Name == "web-0" {
				// cur_serires := tsContainer.SeriesMap[pod.Name]
				snapshot := tsContainer.GetSnapshotOfTimeSeries(pod.Name)
				// mint, maxt := cur_serires.GetMinMaxTime()
				hasNew = true
				mint = Min(snapshot.MinTime, mint)
				maxt = Max(snapshot.MaxTime, maxt)
				// pendLogStr += fmt.Sprintf("[collectMetrics]%v mint,maxt: %v ~ %v statistics: cpu: %v %v mem: %v %v\n", pod.Name,
				// 	snapshot.MinTime, snapshot.MaxTime,
				// 	snapshot.AvgOfCpu,
				// 	snapshot.SampleCntOfCpu,
				// 	snapshot.AvgOfMem,
				// 	snapshot.SampleCntOfMem,
				// )
				// log.Printf("[collectMetrics]%v mint,maxt: %v ~ %v statistics: cpu: %v %v mem: %v %v\n", pod.Name,
				// 	snapshot.MinTime, snapshot.MaxTime,
				// 	snapshot.AvgOfCpu,
				// 	snapshot.SampleCntOfCpu,
				// 	snapshot.AvgOfMem,
				// 	snapshot.SampleCntOfMem,
				// )
			}

		}

		// just print tenant's avg metrics
		if hasNew {
			hasNew = false
			tArr := c.AutoScaleMeta.GetTenantNames()
			for _, tName := range tArr {
				stats, _ := as_meta.ComputeStatisticsOfTenant(tName, tsContainer, "collectMetrics")
				log.Printf("[collectMetrics]Tenant %v statistics: cpu: %v %v mem: %v %v time_range:%v~%v\n", tName,
					stats[0].Avg(),
					stats[0].Cnt(),
					stats[1].Avg(),
					stats[1].Cnt(),
					mint, maxt,
				)
			}
		}
	}

}

func (c *ClusterManager) analyzeMetrics() {
	// TODO implement

	// c.tsContainer.GetSnapshotOfTimeSeries()
	defer c.wg.Done()
	lastTs := int64(0)
	for {
		time.Sleep(100 * time.Millisecond)
		if atomic.LoadInt32(&c.shutdown) != 0 {
			return
		}
		tenants := c.AutoScaleMeta.GetTenants()
		for _, tenant := range tenants {
			if tenant.GetState() == TenantStatePause {
				continue
			}
			cntOfPods := tenant.GetCntOfPods()
			if cntOfPods == 0 {
				log.Printf("[analyzeMetrics] StateResume and tenant.GetCntOfPods() is 0, resume pods, minCntOfPods:%v tenant: %v\n", tenant.MinCntOfPod, tenant.Name)
				c.AutoScaleMeta.ResizePodsOfTenant(0, tenant.MinCntOfPod, tenant.Name, c.tsContainer)
			} else {
				stats, podCpuMap := c.AutoScaleMeta.ComputeStatisticsOfTenant(tenant.Name, c.tsContainer, "analyzeMetrics")
				cpuusage := stats[0].Avg()

				//Mock Metrics
				CoreOfPod := DefaultCoreOfPod
				curTs := time.Now().Unix()
				// cpuusage := MockComputeStatisticsOfTenant(CoreOfPod, cntOfPods, tenant.MaxCntOfPod)
				if lastTs != curTs {
					// log.Printf("[analyzeMetrics]ComputeStatisticsOfTenant, pods Of Tenant %v: %+v\n", tenant.Name, tenant.GetPodNames())
					log.Printf("[analyzeMetrics]ComputeStatisticsOfTenant, Tenant %v , cpu usage: %v %v , PodsCpuMap: %+v \n", tenant.Name,
						stats[0].Avg(), stats[0].Cnt(), podCpuMap)
					// log.Printf("[ComputeStatisticsOfTenant] cpu usage: %v\n", cpuusage)
					lastTs = curTs
				}
				bestPods, _ := ComputeBestPodsInRuleOfCompute(tenant, cpuusage, CoreOfPod)
				if bestPods != -1 && cntOfPods != bestPods {
					log.Printf("[analyzeMetrics] resize pods, from %v to  %v , tenant: %v\n", tenant.GetCntOfPods(), bestPods, tenant.Name)
					c.AutoScaleMeta.ResizePodsOfTenant(cntOfPods, bestPods, tenant.Name, c.tsContainer)
				} else {
					// log.Printf("[analyzeMetrics] pods unchanged cnt:%v, bestCnt:%v, tenant:%v \n", tenant.GetCntOfPods(), bestPods, tenant.Name)
				}
			}
			// tenant.IntoTenant()
		}
	}

}

func Int32Ptr(val int32) *int32 {
	ret := new(int32)
	*ret = int32(val)
	return &val
}

func (c *ClusterManager) Shutdown() {
	log.Println("[ClusterManager]Shutdown")
	atomic.StoreInt32(&c.shutdown, 1)
	c.watchMu.Lock()
	c.watcher.Stop()
	c.watchMu.Unlock()
	c.wg.Wait()
}

func (c *ClusterManager) Pause(tenant string) bool {
	return c.AutoScaleMeta.Pause(tenant)
}

func (c *ClusterManager) Resume(tenant string) bool {
	return c.AutoScaleMeta.Resume(tenant, c.tsContainer)
}

func (c *ClusterManager) watchPodsLoop(resourceVersion string) {
	defer c.wg.Done()
	for {
		if atomic.LoadInt32(&c.shutdown) != 0 {
			return
		}
		labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"app": c.CloneSetName}}
		watcher, err := c.K8sCli.CoreV1().Pods(c.Namespace).Watch(context.TODO(),
			metav1.ListOptions{
				LabelSelector:   labels.Set(labelSelector.MatchLabels).String(),
				ResourceVersion: resourceVersion,
			})

		if err != nil {
			panic(err.Error())
		}

		c.watchMu.Lock()
		c.watcher = watcher
		c.watchMu.Unlock()

		ch := watcher.ResultChan()

		// LISTEN TO CHANNEL
		for {
			e, more := <-ch
			if !more {
				log.Printf("watchPods channel closed\n")
				break
			}
			pod, ok := e.Object.(*v1.Pod)
			if !ok {
				continue
			}
			resourceVersion = pod.ResourceVersion
			switch e.Type {
			case watch.Added:
				c.AutoScaleMeta.UpdatePod(pod)
			case watch.Modified:
				c.AutoScaleMeta.UpdatePod(pod)
			case watch.Deleted:
				c.AutoScaleMeta.HandleK8sDelPodEvent(pod)
			default:
				fallthrough
			case watch.Error, watch.Bookmark: //TODO handle it
				continue
			}
			// log.Printf("act,ns,name,phase,reason,ip,noOfContainer: %v %v %v %v %v %v %v\n", e.Type,
			// 	pod.Namespace,
			// 	pod.Name,
			// 	pod.Status.Phase,
			// 	pod.Status.Reason,
			// 	pod.Status.PodIP,
			// 	len(pod.Status.ContainerStatuses))

		}
	}

}

// func (c *ClusterManager) scanStateOfPods() {
// 	c.AutoScaleMeta.scanStateOfPods()
// }

// ignore error
func (c *ClusterManager) loadPods() string {
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"app": c.CloneSetName}}
	pods, err := c.K8sCli.CoreV1().Pods(c.Namespace).List(context.TODO(),
		metav1.ListOptions{LabelSelector: labels.Set(labelSelector.MatchLabels).String()})
	if err != nil {
		return ""
	}
	resVer := pods.ListMeta.ResourceVersion
	for _, pod := range pods.Items {
		c.AutoScaleMeta.UpdatePod(&pod)
	}
	return resVer
}

// TODO load existed pods
func (c *ClusterManager) initK8sComponents() {
	// create cloneset if not exist
	cloneSetList, err := c.Cli.AppsV1alpha1().CloneSets(c.Namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	log.Printf("list clonneSet: %+v \n", len(cloneSetList.Items))
	found := false
	for _, cloneSet := range cloneSetList.Items {
		if cloneSet.Name == c.CloneSetName {
			found = true
			break
		}
	}
	var retCloneset *v1alpha1.CloneSet
	if !found {
		// volumeName := "tiflash-readnode-data-vol"
		/// TODO ensure one pod one node and fixed nodegroup
		//create cloneSet since there is no desired cloneSet
		cloneSet := v1alpha1.CloneSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.CloneSetName,
				Labels: map[string]string{
					"app": c.CloneSetName,
				}},
			Spec: v1alpha1.CloneSetSpec{
				Replicas: Int32Ptr(int32(c.AutoScaleMeta.PrewarmPods.MaxCntOfPod)),
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": c.CloneSetName,
					}},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": c.CloneSetName,
						},
					},
					// pod anti affinity
					Spec: v1.PodSpec{
						NodeSelector: map[string]string{
							"node.kubernetes.io/instance-type": "m6a.2xlarge", // TODO use a non-hack way to bind readnode pod to specific nodes
						},
						Affinity: &v1.Affinity{
							PodAntiAffinity: &v1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
									{
										TopologyKey: "kubernetes.io/hostname",
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      "app",
													Operator: "In",
													Values:   []string{c.CloneSetName, "autoscale"},
												},
											},
										},
									},
								},
							},
						},
						// container
						Containers: []v1.Container{
							{
								// ENV
								Env: []v1.EnvVar{
									{
										Name: "POD_IP",
										ValueFrom: &v1.EnvVarSource{
											FieldRef: &v1.ObjectFieldSelector{
												FieldPath: "status.podIP",
											},
										},
									},
								},
								Name:            "supervisor",
								Image:           "bestwoody/supervisor:1",
								ImagePullPolicy: "Always",
								// VolumeMounts: []v1.VolumeMount{
								// 	{
								// 		Name:      volumeName,
								// 		MountPath: "/usr/share/nginx/html",
								// 	}},
							},
						},
					},
				},
				// VolumeClaimTemplates: []v1.PersistentVolumeClaim{
				// 	{
				// 		ObjectMeta: metav1.ObjectMeta{
				// 			Name: volumeName,
				// 		},
				// 		Spec: v1.PersistentVolumeClaimSpec{
				// 			AccessModes: []v1.PersistentVolumeAccessMode{
				// 				"ReadWriteOnce",
				// 			},
				// 			Resources: v1.ResourceRequirements{
				// 				Requests: v1.ResourceList{
				// 					"storage": resource.MustParse("20Gi"),
				// 				},
				// 			},
				// 		},
				// 	},
				// },
			},
		}
		log.Println("create clonneSet")
		retCloneset, err = c.Cli.AppsV1alpha1().CloneSets(c.Namespace).Create(context.TODO(), &cloneSet, metav1.CreateOptions{})
	} else {
		log.Println("get clonneSet")
		retCloneset, err = c.Cli.AppsV1alpha1().CloneSets(c.Namespace).Get(context.TODO(), c.CloneSetName, metav1.GetOptions{})
	}
	if err != nil {
		panic(err.Error())
	} else {
		c.CloneSet = retCloneset.DeepCopy()
	}

	// load k8s pods of cloneset
	resVer := c.loadPods()

	// TODO do we need scan stats of pods with incorrect state label
	// c.scanStateOfPods()
	c.AutoScaleMeta.ScanStateOfPods()

	// watch changes of pods
	c.wg.Add(1)
	go c.watchPodsLoop(resVer)
}

// TODO must implement!!! necessary
func (c *ClusterManager) recoverStatesOfPods() {
	log.Println("[ClusterManager] recoverStatesOfPods(): unimplement")
	// c.AutoScaleMeta.recoverStatesOfPods()
}

func initK8sEnv(Namespace string) (config *restclient.Config, K8sCli *kubernetes.Clientset, MetricsCli *metricsv.Clientset, Cli *kruiseclientset.Clientset) {
	config, err := getK8sConfig()
	if err != nil {
		panic(err.Error())
	}
	MetricsCli, err = metricsv.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	K8sCli, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	Cli = kruiseclientset.NewForConfigOrDie(config)

	// create NameSpace if not exsist
	_, err = K8sCli.CoreV1().Namespaces().Get(context.TODO(), Namespace, metav1.GetOptions{})
	if err != nil {
		_, err = K8sCli.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: Namespace,
				Labels: map[string]string{
					"ns": Namespace,
				}}}, metav1.CreateOptions{})
		if err != nil {
			panic(err.Error())
		}
	}
	// _, err = K8sCli.CoreV1().Namespaces().Get(context.TODO(), Namespace, metav1.GetOptions{})
	// if err != nil {
	// 	_, err = K8sCli.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
	// 		ObjectMeta: metav1.ObjectMeta{
	// 			Name: Namespace,
	// 			Labels: map[string]string{
	// 				"ns": Namespace,
	// 			}}}, metav1.CreateOptions{})
	// 	if err != nil {
	// 		panic(err.Error())
	// 	}
	// }
	return config, K8sCli, MetricsCli, Cli
}

// podstat:   init->prewarmed<--->ComputePod

func NewClusterManager() *ClusterManager {
	namespace := "tiflash-autoscale"
	k8sConfig, K8sCli, MetricsCli, Cli := initK8sEnv(namespace)
	ret := &ClusterManager{
		Namespace:     namespace,
		CloneSetName:  "readnode",
		AutoScaleMeta: NewAutoScaleMeta(k8sConfig),
		tsContainer:   NewTimeSeriesContainer(4),
		lstTsMap:      make(map[string]int64),

		K8sCli:     K8sCli,
		MetricsCli: MetricsCli,
		Cli:        Cli,
	}
	ret.initK8sComponents()

	ret.wg.Add(2)
	go ret.collectMetrics()
	go ret.analyzeMetrics()
	return ret
}

// TODO mutex protection
func AddNewPods(c *ClusterManager, cli *kruiseclientset.Clientset, ns string, cloneSet *v1alpha1.CloneSet, from int, delta int) (*v1alpha1.CloneSet, error) {
	// TODO add mutex protection?
	if delta <= 0 {
		return cloneSet, fmt.Errorf("delta <= 0")
	}
	if int32(from) != *cloneSet.Spec.Replicas {
		return cloneSet, fmt.Errorf("int32(from) != *cloneSet.Spec.Replicas")
	}
	newReplicas := new(int32)
	*newReplicas = int32(from + delta)
	cloneSet.Spec.Replicas = newReplicas
	ret, err := cli.AppsV1alpha1().CloneSets(ns).Update(context.TODO(), cloneSet, metav1.UpdateOptions{})
	if err != nil {
		return cloneSet, fmt.Errorf(err.Error())
	} else {
		c.CloneSet = ret.DeepCopy()
		return ret, nil
	}
}

func (c *ClusterManager) AddNewPods(from int, delta int) (*v1alpha1.CloneSet, error) {
	return AddNewPods(c, c.Cli, c.Namespace, c.CloneSet, from, delta)
}

func (c *ClusterManager) Wait() {
	c.wg.Wait()
}
