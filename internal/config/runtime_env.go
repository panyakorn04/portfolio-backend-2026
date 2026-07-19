package config

import (
	"fmt"
	"strconv"
	"strings"
)

type EnvironmentLookup func(string) (string, bool)

// ApplyEnvironmentOverrides maps optional runtime variables that need safe
// typed defaults when absent. Error messages identify only the variable name
// and never include its value.
func ApplyEnvironmentOverrides(c *Config, lookup EnvironmentLookup) error {
	if value, ok := lookup("AI_API_KEY"); ok {
		c.AiApiKey = strings.TrimSpace(value)
	}

	if err := applyPositiveIntOverride(
		"PORTFOLIO_CHAT_SESSION_TTL_HOURS",
		&c.PortfolioChatSessionTTLHours,
		lookup,
	); err != nil {
		return err
	}
	if err := applyPositiveIntOverride(
		"PORTFOLIO_CHAT_MAX_STORED_MESSAGES",
		&c.PortfolioChatMaxStoredMessages,
		lookup,
	); err != nil {
		return err
	}
	return nil
}

func applyPositiveIntOverride(name string, destination *int, lookup EnvironmentLookup) error {
	raw, ok := lookup(name)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*destination = value
	return nil
}
