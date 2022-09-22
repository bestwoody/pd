package autoscale

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"

	"github.com/openkruise/kruise-api/apps/v1alpha1"
	kruiseclientset "github.com/openkruise/kruise-api/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

//TODO mutex protection
type ClusterManager struct {
	Namespace    string
	CloneSetName string
	*AutoScaleMeta
	K8sCli     *kubernetes.Clientset
	MetricsCli *metricsv.Clientset
	Cli        *kruiseclientset.Clientset
	CloneSet   *v1alpha1.CloneSet
}

func Int32Ptr(val int32) *int32 {
	ret := new(int32)
	*ret = int32(val)
	return &val
}

func (c *ClusterManager) initK8sClient() {
	config, err := getK8sConfig()
	if err != nil {
		panic(err.Error())
	}
	c.MetricsCli, err = metricsv.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	c.K8sCli, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	c.Cli = kruiseclientset.NewForConfigOrDie(config)
	_, err = c.K8sCli.CoreV1().Namespaces().Get(context.TODO(), c.Namespace, metav1.GetOptions{})
	if err != nil {
		_, err = c.K8sCli.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.Namespace,
				Labels: map[string]string{
					"ns": c.Namespace,
				}}}, metav1.CreateOptions{})
		if err != nil {
			panic(err.Error())
		}
	}
	cloneSetList, err := c.Cli.AppsV1alpha1().CloneSets(c.Namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("list clonneSet: %+v \n", len(cloneSetList.Items))
	found := false
	for _, cloneSet := range cloneSetList.Items {
		if cloneSet.Name == c.CloneSetName {
			found = true
			break
		}
	}
	if !found {
		volumeName := "tiflash-readnode-data-vol"
		//create cloneSet since there is no desired cloneSet
		cloneSet := v1alpha1.CloneSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: c.CloneSetName,
				Labels: map[string]string{
					"app": c.CloneSetName,
				}},
			Spec: v1alpha1.CloneSetSpec{
				Replicas: Int32Ptr(int32(c.PrewarmPods.MaxCntOfPod)),
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
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Name:  "web",
								Image: "nginx:1.12",
								VolumeMounts: []v1.VolumeMount{
									{
										Name:      volumeName,
										MountPath: "/usr/share/nginx/html",
									}},
							},
						},
					},
				},
				VolumeClaimTemplates: []v1.PersistentVolumeClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: volumeName,
						},
						Spec: v1.PersistentVolumeClaimSpec{
							AccessModes: []v1.PersistentVolumeAccessMode{
								"ReadWriteOnce",
							},
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									"storage": resource.MustParse("20Gi"),
								},
							},
						},
					},
				},
			},
		}
		fmt.Println("create clonneSet")
		c.CloneSet, err = c.Cli.AppsV1alpha1().CloneSets(c.Namespace).Create(context.TODO(), &cloneSet, metav1.CreateOptions{})
	} else {
		fmt.Println("get clonneSet")
		c.CloneSet, err = c.Cli.AppsV1alpha1().CloneSets(c.Namespace).Get(context.TODO(), c.CloneSetName, metav1.GetOptions{})
	}
	if err != nil {
		panic(err.Error())
	}
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"app": c.CloneSetName}}
	pods, err := c.K8sCli.CoreV1().Pods(c.Namespace).List(context.TODO(),
		metav1.ListOptions{LabelSelector: labels.Set(labelSelector.MatchLabels).String()})
	if err != nil {
		panic(err.Error())
	}
	for _, pod := range pods.Items {
		fmt.Println(pod.Name, pod.Status.PodIP)
	}
}

func NewClusterManager() *ClusterManager {
	ret := &ClusterManager{Namespace: "tiflash-autoscale", CloneSetName: "readnode", AutoScaleMeta: NewAutoScaleMeta()}
	ret.initK8sClient()
	return ret
}

//TODO mutex protection
func AddNewPods(cli *kruiseclientset.Clientset, ns string, cloneSet *v1alpha1.CloneSet, from int, delta int) (*v1alpha1.CloneSet, error) {
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
		return ret, nil
	}
}

func (c *ClusterManager) AddNewPods(from int, delta int) (*v1alpha1.CloneSet, error) {
	return AddNewPods(c.Cli, c.Namespace, c.CloneSet, from, delta)
}
