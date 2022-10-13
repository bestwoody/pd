package main

import (
	"encoding/json"
	"fmt"
	"github.com/tikv/pd/autoscale"
	"io"
	"log"
	"net/http"
	"os"
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

var (
	cm *autoscale.ClusterManager
)

func SetStateServer(w http.ResponseWriter, req *http.Request) {
	//state := req.FormValue("state")
}

func GetStateServer(w http.ResponseWriter, req *http.Request) {
	tenantName := req.FormValue("tenantName")
	if tenantName == "" {
		tenantName = "t1"
	}
	ret := GetStateResult{}
	tenants := cm.AutoScaleMeta.GetTenants()
	for _, tenant := range tenants {
		if tenant.Name == tenantName {
			ret.NumOfRNs = tenant.GetCntOfPods()
			state := tenant.GetState()
			if state == autoscale.TenantStateResumed {
				ret.State = "resumed"
			} else if state == autoscale.TenantStateResuming {
				ret.State = "resuming"
			} else if state == autoscale.TenantStatePaused {
				ret.State = "paused"
			} else if state == autoscale.TenantStatePausing {
				ret.State = "pausing"
			}
			retJson, _ := json.Marshal(ret)
			io.WriteString(w, string(retJson))
			return
		}
	}
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
