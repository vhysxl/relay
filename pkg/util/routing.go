package util

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/vhysxl/relay/pkg/errdefs"
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

func ValidateActivity(activity string) error {
	if strings.TrimSpace(activity) == "" {
		return errdefs.ErrEmptyActivity
	}
	if !IsRoutingIdentifier(activity) {
		return errdefs.ErrInvalidActivity
	}
	return nil
}

// NormalizeInput supplies an empty object only for an omitted input; JSON null is preserved.
func NormalizeInput(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return json.RawMessage(`{}`)
	}
	return input
}

// DurationSeconds rounds a positive duration up to whole seconds.
func DurationSeconds(duration time.Duration) int64 { return int64((duration-1)/time.Second) + 1 }
