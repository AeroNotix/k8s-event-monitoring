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

