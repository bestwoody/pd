package autoscale

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/tikv/pd/autoscale"
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
	TenantStateResumedString  = "available"
	TenantStateResumingString = "resuming"
	TenantStatePausedString   = "paused"
	TenantStatePausingString  = "pausing"
)

var (
	cm *autoscale.ClusterManager
)

func SetStateServer(w http.ResponseWriter, req *http.Request) {
	tenantName := req.FormValue("tenantName")
	ret := SetStateResult{}
	if tenantName == "" {
		tenantName = "t1"
	}
	flag, currentState, _ := cm.AutoScaleMeta.GetTenantState(tenantName)
	if !flag {
		ret.HasError = 1
		ret.ErrorInfo = "get state failed"
		retJson, _ := json.Marshal(ret)
		io.WriteString(w, string(retJson))
		return
	}
	state := req.FormValue("state")
	if currentState == autoscale.TenantStatePaused && state == "resume" {
		flag = cm.Resume(tenantName)
		if !flag {
			ret.HasError = 1
			ret.ErrorInfo = "resume failed"
			retJson, _ := json.Marshal(ret)
			io.WriteString(w, string(retJson))
			return
		}
		_, currentState, _ = cm.AutoScaleMeta.GetTenantState(tenantName)
		ret.State = convertStateString(currentState)
		retJson, _ := json.Marshal(ret)
		io.WriteString(w, string(retJson))
		return
	} else if currentState == autoscale.TenantStateResumed && state == "pause" {
		flag = cm.Pause(tenantName)
		if !flag {
			ret.HasError = 1
			ret.ErrorInfo = "pause failed"
			retJson, _ := json.Marshal(ret)
			io.WriteString(w, string(retJson))
			return
		}
		_, currentState, _ = cm.AutoScaleMeta.GetTenantState(tenantName)
		ret.State = convertStateString(currentState)
		retJson, _ := json.Marshal(ret)
		io.WriteString(w, string(retJson))
		return
	}
	ret.HasError = 1
	ret.State = convertStateString(currentState)
	ret.ErrorInfo = "invalid set state"
	retJson, _ := json.Marshal(ret)
	io.WriteString(w, string(retJson))
	return
}

func GetStateServer(w http.ResponseWriter, req *http.Request) {
	tenantName := req.FormValue("tenantName")
	if tenantName == "" {
		tenantName = "t1"
	}
	ret := GetStateResult{}
	flag, state, numOfRNs := cm.AutoScaleMeta.GetTenantState(tenantName)
	if !flag {
		ret.HasError = 1
		ret.ErrorInfo = "get state failed"
		retJson, _ := json.Marshal(ret)
		io.WriteString(w, string(retJson))
		return
	}
	ret.NumOfRNs = numOfRNs
	ret.State = convertStateString(state)
	retJson, _ := json.Marshal(ret)
	io.WriteString(w, string(retJson))
}

func convertStateString(state int32) string {
	if state == autoscale.TenantStateResumed {
		return TenantStateResumedString
	} else if state == autoscale.TenantStateResuming {
		return TenantStateResumingString
	} else if state == autoscale.TenantStatePaused {
		return TenantStatePausedString
	}
	return TenantStatePausingString
}

func RunAutoscaleHttpServer() {
	// autoscale.HardCodeEnvPdAddr = os.Getenv("PD_ADDR")
	// autoscale.HardCodeEnvTidbStatusAddr = os.Getenv("TIDB_STATUS_ADDR")
	// fmt.Printf("env.PD_ADDR: %v\n", autoscale.HardCodeEnvPdAddr)
	// fmt.Printf("env.TIDB_STATUS_ADDR: %v\n", autoscale.HardCodeEnvTidbStatusAddr)
	// cm = autoscale.NewClusterManager()

	http.HandleFunc("/setstate", SetStateServer)
	http.HandleFunc("/getstate", GetStateServer)
	err := http.ListenAndServe(":8081", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
