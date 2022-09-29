package autoscale

import (
	"container/list"
	"sync"
	"time"
)

func Max(a int64, b int64) int64 {
	if a > b {
		return a
	} else {
		return b
	}
}

func Min(a int64, b int64) int64 {
	if a < b {
		return a
	} else {
		return b
	}
}

func MaxInt(a int, b int) int {
	if a > b {
		return a
	} else {
		return b
	}
}

func MinInt(a int, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

type TimeValues struct {
	time   int64
	values []float64
	window time.Duration
}

type AvgSigma struct {
	sum float64
	cnt int64
}

type SimpleTimeSeries struct {
	series     *list.List // elem type: TimeValues
	Statistics []AvgSigma
	// min_time   int64
	max_time int64
	// cap    int
}

func (c *SimpleTimeSeries) Reset() {
	for c.series.Len() > 0 {
		c.series.Remove(c.series.Front())
	}
	for _, v := range c.Statistics {
		v.Reset()
	}
	c.max_time = 0
}

func (c *SimpleTimeSeries) Cpu() *AvgSigma {
	return &c.Statistics[0]
}

func (c *SimpleTimeSeries) Mem() *AvgSigma {
	return &c.Statistics[1]
}

func (cur *AvgSigma) Reset() {
	cur.cnt = 0
	cur.sum = 0
}

func (cur *AvgSigma) Sub(v float64) {
	cur.cnt--
	cur.sum -= v
}

func (cur *AvgSigma) Add(v float64) {
	cur.cnt++
	cur.sum += v
}

func (cur *AvgSigma) Avg() float64 {
	return cur.sum / float64(cur.cnt)
}

func (cur *AvgSigma) Cnt() int64 {
	return cur.cnt
}

func (cur *AvgSigma) Merge(o *AvgSigma) {
	cur.cnt += o.cnt
	cur.sum += o.sum
}

func Sub(cur []AvgSigma, values []float64) {
	for i, value := range values {
		cur[i].Sub(value)
	}
}

func Add(cur []AvgSigma, values []float64) {
	for i, value := range values {
		cur[i].Add(value)
	}
}

func Merge(cur []AvgSigma, o []AvgSigma) {
	if o == nil {
		return
	}
	for i, value := range o {
		cur[i].Merge(&value)
	}
}

func Avg(cur []AvgSigma) []float64 {
	ret := make([]float64, 3)
	for _, elem := range cur {
		ret = append(ret, elem.Avg())
	}
	return ret
}

type StatsOfTimeSeries struct {
	AvgOfCpu       float64
	AvgOfMem       float64
	SampleCntOfCpu int64
	SampleCntOfMem int64
	MinTime        int64
	MaxTime        int64
}

type TimeSeriesContainer struct {
	seriesMap          map[string]*SimpleTimeSeries
	cap_of_each_series int
	mu                 sync.Mutex
}

func NewTimeSeriesContainer(cap_of_each_series int) *TimeSeriesContainer {
	return &TimeSeriesContainer{
		seriesMap:          make(map[string]*SimpleTimeSeries),
		cap_of_each_series: cap_of_each_series}
}

func (c *TimeSeriesContainer) GetStatisticsOfPod(podname string) []AvgSigma {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.seriesMap[podname]
	if !ok {
		return nil
	}
	ret := make([]AvgSigma, CapacityOfStaticsAvgSigma)
	Merge(ret, v.Statistics)
	return ret
}

func (c *TimeSeriesContainer) GetSnapshotOfTimeSeries(podname string) *StatsOfTimeSeries {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.seriesMap[podname]
	if !ok {
		return nil
	}
	minTime, maxTime := v.getMinMaxTime()

	return &StatsOfTimeSeries{AvgOfCpu: v.Cpu().Avg(),
		SampleCntOfCpu: v.Cpu().Cnt(),
		AvgOfMem:       v.Mem().Avg(),
		SampleCntOfMem: v.Mem().Cnt(),
		MinTime:        minTime, MaxTime: maxTime}
}

func (c *TimeSeriesContainer) ResetMetricsOfPod(podname string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.seriesMap[podname]
	if ok {
		v.Reset()
	}
}

func (cur *SimpleTimeSeries) getMinMaxTime() (int64, int64) {
	min_time := cur.series.Front().Value.(*TimeValues).time
	return min_time, cur.max_time
}

func (cur *SimpleTimeSeries) append(time int64, values []float64, cap int) {
	cur.series.PushBack(
		&TimeValues{
			time:   time,
			values: values,
		})
	if cur.max_time == 0 {
		cur.max_time = time
	} else {
		cur.max_time = Max(cur.max_time, time)
	}
	Add(cur.Statistics, values)
	for cur.series.Len() > cap {
		Sub(cur.Statistics, cur.series.Front().Value.(*TimeValues).values)
		cur.series.Remove(cur.series.Front())
	}
}

func (cur *TimeSeriesContainer) Insert(key string, time int64, values []float64) {
	cur.mu.Lock()
	defer cur.mu.Unlock()
	val, ok := cur.seriesMap[key]
	if !ok {
		val = &SimpleTimeSeries{
			series:     list.New(),
			Statistics: make([]AvgSigma, CapacityOfStaticsAvgSigma),
		}
		cur.seriesMap[key] = val
	}
	val.append(time, values, cur.cap_of_each_series)
}
