package autoscale

import (
	"container/list"
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

type TimeSeriesContainer struct {
	SeriesMap          map[string]*SimpleTimeSeries
	cap_of_each_series int
}

func NewTimeSeriesContainer(cap_of_each_series int) *TimeSeriesContainer {
	return &TimeSeriesContainer{
		SeriesMap:          make(map[string]*SimpleTimeSeries),
		cap_of_each_series: cap_of_each_series}
}

func (cur *SimpleTimeSeries) GetMinMaxTime() (int64, int64) {
	min_time := cur.series.Front().Value.(*TimeValues).time
	return min_time, cur.max_time
}

func (cur *SimpleTimeSeries) Append(time int64, values []float64, cap int) {
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
	val, ok := cur.SeriesMap[key]
	if !ok {
		val = &SimpleTimeSeries{
			series:     list.New(),
			Statistics: make([]AvgSigma, 6),
		}
		cur.SeriesMap[key] = val
	}
	val.Append(time, values, cur.cap_of_each_series)
}
