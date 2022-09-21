package autoscale

import "sync"

func ComputeBestPodsInRulePM(tenantDesc *TenantDesc, cpuusage float64, coreOfPod int, mu *sync.Mutex) (int, bool) {
	if tenantDesc == nil {
		return -1, false
	}
	lowLimit := float64(coreOfPod) * DefaultLowerLimit
	upLimit := float64(coreOfPod) * DefaultHigherLimit
	if cpuusage >= lowLimit && cpuusage <= upLimit {
		return -1, false
	} else {
		mu.Lock()
		oldCntOfPods := len(tenantDesc.Pods)
		minCntOfPods := tenantDesc.MinCntOfPod
		maxCntOfPods := tenantDesc.MaxCntOfPod
		mu.Unlock()
		if cpuusage > upLimit {
			return MinInt(oldCntOfPods+1, maxCntOfPods), true
			// return +1,false
		} else if cpuusage < lowLimit {
			return MaxInt(oldCntOfPods-1, minCntOfPods), true
		} else {
			return -1, false
		}
	}
}
