package sailor

import (
	"encoding/json"
	"net/http"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// consumerReportBody is the wire shape of a consumer report (CONS-HTTP-11 to CONS-HTTP-14).
type consumerReportBody struct {
	Consumer string            `json:"consumer"`
	Version  string            `json:"version,omitempty"`
	Digest   string            `json:"digest"`
	Details  map[string]string `json:"details,omitempty"`
}

// reportAsync sends a report in a goroutine the consumer owns (CONS-RPT-4).
// Without a token nothing is sent; Start has already logged that once.
func (c *Consumer) reportAsync(r *resource, version, digest string) {
	if c.conn.token == "" {
		return
	}
	c.spawn(func() { c.postReport(r, version, digest) })
}

// postReport sends one consumer report and logs the outcome (CONS-RPT-5, CONS-HTTP-17).
func (c *Consumer) postReport(r *resource, version, digest string) {
	body, err := json.Marshal(consumerReportBody{
		Consumer: c.identity(),
		Version:  version,
		Digest:   digest,
		Details:  c.reportDetails(),
	})
	if err != nil {
		c.emit(opts.LevelWarn, "report.failed", "resource", r.name, "status", 0, "message", err.Error())
		return
	}

	target := c.resourceURL(r.name) + "/consumer-report"
	resp, err := c.do(c.ctx, http.MethodPost, target, body,
		map[string]string{"Content-Type": "application/json"}, true, c.reportTimeout)
	if err != nil {
		if c.ctx.Err() == nil {
			c.emit(opts.LevelWarn, "report.failed", "resource", r.name, "status", 0, "message", truncate(err.Error(), 4096))
		}
		return
	}
	if resp.status < 200 || resp.status >= 300 {
		c.emit(opts.LevelWarn, "report.failed",
			"resource", r.name, "status", resp.status, "message", truncate(string(resp.body), 4096))
		return
	}
	c.emit(opts.LevelDebug, "report.acknowledged", "resource", r.name, "status", resp.status)
}

// identity is the host name, or "unknown" (CONS-RPT-6).
func (c *Consumer) identity() string {
	if h, err := c.deps.hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

// reportDetails carries the optional pod identity; empty values are omitted (CONS-RPT-7).
func (c *Consumer) reportDetails() map[string]string {
	out := map[string]string{}
	for key, env := range map[string]string{"pod_name": "POD_NAME", "pod_ip": "POD_IP", "node_name": "NODE_NAME"} {
		if v := c.getenv(env); v != "" {
			out[key] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// heartbeatLoop re-reports the current state on a fixed interval (CONS-RPT-2).
func (c *Consumer) heartbeatLoop(r *resource) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.clock.After(heartbeatInterval):
		}
		if loaded, version, digest := r.snapshot(); loaded {
			c.reportAsync(r, version, digest)
		}
	}
}
