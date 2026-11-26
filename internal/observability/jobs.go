package observability

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RegisterBuildInfo exposes niro_build_info{version} = 1, so dashboards and
// alerts can tell which release is running.
func RegisterBuildInfo(reg prometheus.Registerer, version string) error {
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "niro_build_info", Help: "The running Niro version."}, []string{"version"})
	g.WithLabelValues(version).Set(1)
	return reg.Register(g)
}

// Jobs records how background jobs went, so an alert can fire when one keeps
// failing or stops running: niro_job_last_success_timestamp_seconds and
// niro_job_failures_total, both by job name.
type Jobs struct {
	lastSuccess *prometheus.GaugeVec
	failures    *prometheus.CounterVec
	runs        *prometheus.CounterVec
}

// NewJobs registers the job metrics.
func NewJobs(reg prometheus.Registerer) (*Jobs, error) {
	j := &Jobs{
		lastSuccess: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "niro_job_last_success_timestamp_seconds", Help: "When each background job last finished without error.",
		}, []string{"job"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_job_failures_total", Help: "Background job runs that failed, by job.",
		}, []string{"job"}),
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_job_runs_total", Help: "Background job runs, by job.",
		}, []string{"job"}),
	}
	if reg != nil {
		for _, c := range []prometheus.Collector{j.lastSuccess, j.failures, j.runs} {
			if err := reg.Register(c); err != nil {
				return nil, err
			}
		}
	}
	return j, nil
}

// Done records one run of a job. A nil *Jobs does nothing.
func (j *Jobs) Done(job string, err error) {
	if j == nil {
		return
	}
	j.runs.WithLabelValues(job).Inc()
	if err != nil {
		j.failures.WithLabelValues(job).Inc()
		return
	}
	j.lastSuccess.WithLabelValues(job).Set(float64(time.Now().Unix()))
}

// Reporter returns a callback for one job, for code that takes func(error).
func (j *Jobs) Reporter(job string) func(error) {
	return func(err error) { j.Done(job, err) }
}
