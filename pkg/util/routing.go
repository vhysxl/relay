package util

import (
	"encoding/json"
	"github.com/vhysxl/relay/pkg/errdefs"
	"regexp"
	"strings"
)

var routingIdentifierRegex = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

func IsRoutingIdentifier(value string) bool { return routingIdentifierRegex.MatchString(value) }

func ValidateQueueName(queue string) error {
	if strings.TrimSpace(queue) == "" {
		return errdefs.ErrEmptyQueueName
	}
	if !IsRoutingIdentifier(queue) {
		return errdefs.ErrInvalidQueueName
	}
	return nil
}

// QueueFromConfig reads explicit routing without assigning a default queue.
func QueueFromConfig(raw json.RawMessage) (string, error) {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil || config == nil {
		return "", errdefs.ErrInvalidStepConfig
	}
	var queue string
	if err := json.Unmarshal(config["queue"], &queue); err != nil {
		if len(config["queue"]) == 0 {
			return "", errdefs.ErrEmptyQueueName
		}
		return "", errdefs.ErrInvalidQueueName
	}
	if err := ValidateQueueName(queue); err != nil {
		return "", err
	}
	return queue, nil
}
