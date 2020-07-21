package alerting

import (
	"log"

	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/spf13/viper"
)

// Alerter represents something that can trigger an alert to an
// external service. For example, to slack, or opsgenie.
type Alerter interface {
	Alert(message string) error
}

// Registry holds all the available alerters to be used by deadman
// switches.
type Registry struct {
	alerters map[string]Alerter
}

// NewRegistry simply allocates a new registry with all fields
// properly created.
func NewRegistry() Registry {
	return Registry{
		alerters: make(map[string]Alerter),
	}
}

func alerterFromRawConfig(raw map[string]string) Alerter {
	switch raw["type"] {
	case "slack":
		return NewSlackAlerter(raw["webhook"], raw["channel"])
	case "log":
		return NewLogAlerter()
	}
	panic("unknown alerter config!")
}

// NewRegistryFromConfig will create a Registry from a viper/json
// config file.
func NewRegistryFromConfig(viper *viper.Viper) Registry {
	r := NewRegistry()
	if rawalerters, ok := viper.Get("alerters").([]interface{}); ok {
		for _, rawsw := range rawalerters {
			sw := make(map[string]string)
			for k, v := range rawsw.(map[string]interface{}) {
				sw[k] = v.(string)
			}
			r.AddAlerter(sw["name"], alerterFromRawConfig(sw))
		}
		return r
	}
	panic("invalid alerter config!")
}

// AddAlerter adds an alerter to the registry.
func (r *Registry) AddAlerter(name string, a Alerter) {
	if _, ok := r.alerters[name]; ok {
		panic("duplicate alert name")
	}
	r.alerters[name] = a
}

// GetAlerter finds the alerter associated with a name.
func (r *Registry) GetAlerter(name string) Alerter {
	return r.alerters[name]
}

// SlackAlerter is an alerter which sends its alert messages to a
// slack channel, over a webhook.
type SlackAlerter struct {
	webhookURL string
	channel    string
}

// NewSlackAlerter creates a slack alerter with the correct webhooks,
// channel and returns it as the Alerter interface.
func NewSlackAlerter(webhookURL, channel string) Alerter {
	return SlackAlerter{
		webhookURL: webhookURL,
		channel:    channel,
	}
}

// Alert implements the Alerter interface.q
func (a SlackAlerter) Alert(message string) error {
	SendMessage(
		a.webhookURL,
		a.channel,
		message,
	)
	return nil
}

// SendMessage ... sends a message to slack?
func SendMessage(webhookURL, channel, message string) {
	payload := slack.Payload{
		Text:      message,
		Username:  "DeadMansSwitch",
		Channel:   channel,
		IconEmoji: ":skull_and_crossbones:",
	}
	err := slack.Send(webhookURL, "", payload)
	if len(err) > 0 {
		log.Printf("error: %s\n", err)
	}
}

// LogAlerter is a dead simple alerter which simply logs to console.
type LogAlerter bool

// NewLogAlerter is just a constructor, dead useful docstring
// this. Love linters.
func NewLogAlerter() Alerter {
	return LogAlerter(false)
}

// Alert implements the Alerter interface for LogAlerter.
func (la LogAlerter) Alert(message string) error {
	log.Println(message)
	return nil
}
