package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/tikv/pd/autoscale"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/azure"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
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

// func GetResourcesDynamically(dynamic dynamic.Interface, ctx context.Context,
// 	group string, version string, resource string, namespace string) (
// 	[]unstructured.Unstructured, error) {

// 	resourceId := schema.GroupVersionResource{
// 		Group:    group,
// 		Version:  version,
// 		Resource: resource,
// 	}
// 	list, err := dynamic.Resource(resourceId).Namespace(namespace).
// 		List(ctx, metav1.ListOptions{})

// 	if err != nil {
// 		return nil, err
// 	}

// 	return list.Items, nil
// }

// func ExampleGetResourcesDynamically() {
// 	ctx := context.Background()
// 	config := ctrl.GetConfigOrDie()
// 	dynamic := dynamic.NewForConfigOrDie(config)

// 	namespace := "default"
// 	items, err := GetResourcesDynamically(dynamic, ctx,
// 		"apps.kruise.io", "v1alpha1", "clonesets", namespace)
// 	if err != nil {
// 		fmt.Println(err)
// 	} else {
// 		for _, item := range items {
// 			fmt.Printf("%+v\n", item)
// 		}
// 	}
// }

// func main() {
// 	cl, err := client.New(config.GetConfigOrDie(), client.Options{})
// 	if err != nil {
// 		fmt.Println("failed to create client")
// 		os.Exit(1)
// 	}

// 	podList := &corev1.PodList{}

// 	err = cl.List(context.Background(), podList, client.InNamespace("default"))
// 	if err != nil {
// 		fmt.Printf("failed to list pods in namespace default: %v\n", err)
// 		os.Exit(1)
// 	} else {
// 		for _, pod := range podList.Items {
// 			fmt.Printf("%+v\n", pod)
// 		}
// 	}
// }

func main() {

	config, err := outsideConfig()

	// // creates the in-cluster config
	// config, err := rest.InClusterConfig()

	if err != nil {
		panic(err.Error())
	}
	tsContainer := autoscale.NewTimeSeriesContainer(5)
	mclientset, err := metricsv.NewForConfig(config)
	as_meta := autoscale.NewAutoScaleMeta()
	as_meta.AddPod("web-0")
	as_meta.AddPod("web-1")
	as_meta.AddPod("web-2")
	as_meta.AddPod("hello-node-7c7c59b7cb-6bsjg")
	as_meta.SetupTenantWithDefaultArgs("t1")
	as_meta.SetupTenantWithDefaultArgs("t2")
	as_meta.UpdateTenant("web-0", "t1")
	as_meta.UpdateTenant("web-1", "t1")
	as_meta.UpdateTenant("web-2", "t1")
	as_meta.UpdateTenant("hello-node-7c7c59b7cb-6bsjg", "t2")
	// var lstTs int64
	lstTsMap := make(map[string]int64)
	hasNew := false
	for {
		podMetricsList, err := mclientset.MetricsV1beta1().PodMetricses("default").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}

		for _, pod := range podMetricsList.Items {
			// if pod.Name == "web-0" {
			// 	fmt.Printf("podmetrics: %v \n", pod)
			// }
			lstTs, ok := lstTsMap[pod.Name]
			if !ok || pod.Timestamp.Unix() != lstTs {
				tsContainer.Insert(pod.Name, pod.Timestamp.Unix(),
					[]float64{
						pod.Containers[0].Usage.Cpu().AsApproximateFloat64(),
						pod.Containers[0].Usage.Memory().AsApproximateFloat64(),
					})
				lstTsMap[pod.Name] = pod.Timestamp.Unix()

				// if pod.Name == "web-0" {
				cur_serires := tsContainer.SeriesMap[pod.Name]
				mint, maxt := cur_serires.GetMinMaxTime()
				hasNew = true
				fmt.Printf("%v mint,maxt: %v ~ %v\n", pod.Name, mint, maxt)
				fmt.Printf("%v statistics: cpu: %v %v mem: %v %v\n", pod.Name,
					cur_serires.Statistics[0].Avg(),
					cur_serires.Statistics[0].Cnt(),
					cur_serires.Statistics[1].Avg(),
					cur_serires.Statistics[1].Cnt(),
				)
				// }
			}

		}
		if hasNew {
			hasNew = false
			tArr := []string{"t1", "t2"}
			for _, tName := range tArr {
				stats1 := as_meta.ComputeStatisticsOfTenant(tName, tsContainer)
				fmt.Printf("[Tenant]%v statistics: cpu: %v %v mem: %v %v\n", tName,
					stats1[0].Avg(),
					stats1[0].Cnt(),
					stats1[1].Avg(),
					stats1[1].Cnt(),
				)
			}
		}
		// v, ok := as_meta.PodDescMap[]
		// fmt.Printf("Podmetrics: %v \n", podMetricsList)
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	// clientset.AppsV1().
	if err != nil {
		panic(err.Error())
	}
	for {
		// get pods in all the namespaces by omitting namespace
		// Or specify namespace to get pods in particular namespace
		pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			panic(err.Error())
		}
		fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

		// Examples for error handling:
		// - Use helper functions e.g. errors.IsNotFound()
		// - And/or cast to StatusError and use its properties like e.g. ErrStatus.Message
		_, err = clientset.CoreV1().Pods("default").Get(context.TODO(), "example-xxxxx", metav1.GetOptions{})
		if errors.IsNotFound(err) {
			fmt.Printf("Pod example-xxxxx not found in default namespace\n")
		} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
			fmt.Printf("Error getting pod %v\n", statusError.ErrStatus.Message)
		} else if err != nil {
			panic(err.Error())
		} else {
			fmt.Printf("Found example-xxxxx pod in default namespace\n")
		}

		time.Sleep(10 * time.Second)
	}
}
