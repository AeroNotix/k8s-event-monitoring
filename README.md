k8s-events-monitoring
---------------------

This application exists to sit inside your Kubernetes cluster and
listen to specific events happening to pods turning those events into
alerts.

Supported events:

* OOMKilled
* Pod killed due to liveness checks failed.

Supported alerters:

* Slack
* Log

Example Configuration
---------------------

```json
{
	"alerters": [
		{
			"handles": "oomkill",
			"type": "slack",
			"templatePath": "/etc/k8s-event-monitoring/slack.tpl"
		},
		{
			"handles": "healthchecks",
			"type": "log"
		}
	]
}
```

Why not Prometheus monitoring?
------------------------------

Very good question!

The first issue is that creating reliable alerts over pod restart
counts is difficult. Creating alerts on OOMKilled pods is also very
tricky, and may lead to no alerts:
https://github.com/kubernetes/kube-state-metrics/pull/535

Initially I did start off using the various kube-state-metrics to
achieve alerts based on particular events but I found that the above
issue with OOMKilled pods coupled with spurious alerts when either
kube-state-metrics was restarted, or crashed, along with the fact that
I need to know about pods restarting, or oom'ing without being
dependant on a slew of monitoring and alerting infr, led to this.
