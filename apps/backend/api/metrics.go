package api

import (
	"expvar"
	"fmt"
	"net/http"
	"strings"
)

type MetricsHandler struct{}

func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

func (h *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var b strings.Builder
	expvar.Do(func(kv expvar.KeyValue) {
		name := sanitizeMetricName(kv.Key)
		switch v := kv.Value.(type) {
		case *expvar.Int:
			fmt.Fprintf(&b, "# HELP %s %s\n", name, kv.Key)
			fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
			fmt.Fprintf(&b, "%s %s\n", name, v.String())
		case *expvar.Float:
			fmt.Fprintf(&b, "# HELP %s %s\n", name, kv.Key)
			fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
			fmt.Fprintf(&b, "%s %s\n", name, v.String())
		case *expvar.String:
			// skip string metrics
		case *expvar.Map:
			fmt.Fprintf(&b, "# HELP %s %s\n", name, kv.Key)
			fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
			v.Do(func(entry expvar.KeyValue) {
				labelKey := sanitizeMetricName(entry.Key)
				switch ev := entry.Value.(type) {
				case *expvar.Int:
					fmt.Fprintf(&b, "%s{key=%q} %s\n", name, labelKey, ev.String())
				case *expvar.Float:
					fmt.Fprintf(&b, "%s{key=%q} %s\n", name, labelKey, ev.String())
				}
			})
		default:
			// skip Func and other types
		}
		b.WriteByte('\n')
	})

	_, _ = w.Write([]byte(b.String()))
}

func sanitizeMetricName(s string) string {
	s = strings.ReplaceAll(s, ".", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, "/", "_")
	return strings.TrimPrefix(s, "_")
}
