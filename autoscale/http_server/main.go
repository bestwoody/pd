package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tikv/pd/autoscale"
	"io"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type SetStateResult struct {
	HasError  int    `json:"hasError"`
	ErrorInfo string `json:"errorInfo"`
	State     string `json:"state"`
}

type GetStateResult struct {
	HasError  int    `json:"hasError"`
	ErrorInfo string `json:"errorInfo"`
	State     string `json:"state"`
	NumOfRNs  int    `json:"numOfRNs"`
}

const (
	TenantStateResumedString  = "resumed"
	TenantStateResumingString = "resuming"
	TenantStatePausedString   = "paused"
	TenantStatePausingString  = "pausing"
)

var (
	cm *autoscale.ClusterManager
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

func SetStateServer(w http.ResponseWriter, req *http.Request) {
	tenantName := req.FormValue("tenantName")
	ret := SetStateResult{}
	if tenantName == "" {
		tenantName = "t1"
	}
	state := req.FormValue("state")
	// TODO: return logic
	if state != "resume" && state != "pause" {
		ret.HasError = 1
		ret.ErrorInfo = "invalid state"
		retJson, _ := json.Marshal(ret)
		io.WriteString(w, string(retJson))
		return
	}
	var flag bool
	if state == "resume" {
		flag = cm.Resume(tenantName)
	} else if state == "pause" {
		flag = cm.Pause(tenantName)
	}
	//current_state = Get
	if !flag {

	}
}

func GetStateServer(w http.ResponseWriter, req *http.Request) {
	tenantName := req.FormValue("tenantName")
	if tenantName == "" {
		tenantName = "t1"
	}
	ret := GetStateResult{}
	//TODO find the target tenant
	//ret.NumOfRNs = tenant.GetCntOfPods()
	//state := tenant.GetState()
	//if state == autoscale.TenantStateResumed {
	//	ret.State = TenantStateResumedString
	//} else if state == autoscale.TenantStateResuming {
	//	ret.State = TenantStateResumingString
	//} else if state == autoscale.TenantStatePaused {
	//	ret.State = TenantStatePausedString
	//} else if state == autoscale.TenantStatePausing {
	//	ret.State = TenantStatePausingString
	//}
	//retJson, _ := json.Marshal(ret)
	//io.WriteString(w, string(retJson))

	ret.HasError = 1
	ret.ErrorInfo = "tenant not found"
	retJson, _ := json.Marshal(ret)
	io.WriteString(w, string(retJson))
}

func main() {
	autoscale.HardCodeEnvPdAddr = os.Getenv("PD_ADDR")
	autoscale.HardCodeEnvTidbStatusAddr = os.Getenv("TIDB_STATUS_ADDR")
	fmt.Printf("env.PD_ADDR: %v\n", autoscale.HardCodeEnvPdAddr)
	fmt.Printf("env.TIDB_STATUS_ADDR: %v\n", autoscale.HardCodeEnvTidbStatusAddr)
	cm = autoscale.NewClusterManager()

	http.HandleFunc("/setstate", SetStateServer)
	http.HandleFunc("/getstate", GetStateServer)
	err := http.ListenAndServe(":8081", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
