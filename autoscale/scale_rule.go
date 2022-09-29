package autoscale

import "sync"

func ComputeBestPodsInRuleOfPM(tenantDesc *TenantDesc, cpuusage float64, coreOfPod int, mu *sync.Mutex) (int, int /*delta*/) {
	if tenantDesc == nil {
		return -1, 0
	}
	lowLimit := float64(coreOfPod) * DefaultLowerLimit
	upLimit := float64(coreOfPod) * DefaultHigherLimit
	if cpuusage >= lowLimit && cpuusage <= upLimit {
		return -1, 0
	} else {
		mu.Lock()
		oldCntOfPods := len(tenantDesc.pods)
		minCntOfPods := tenantDesc.MinCntOfPod
		maxCntOfPods := tenantDesc.MaxCntOfPod
		mu.Unlock()
		if cpuusage > upLimit {
			ret := MinInt(oldCntOfPods+1, maxCntOfPods)
			return ret, ret - oldCntOfPods
		} else if cpuusage < lowLimit {
			ret := MaxInt(oldCntOfPods-1, minCntOfPods)
			return ret, ret - oldCntOfPods
		} else {
			return -1, 0
		}
	}
}
