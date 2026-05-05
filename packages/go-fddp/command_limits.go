package fddp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type CommandLimits struct {
	MaxBodyBytes  int
	MaxInputBytes int
	MaxInputDepth int
	MaxInputNodes int
	Timeout       time.Duration
}

func DefaultCommandLimits() CommandLimits {
	return CommandLimits{
		MaxBodyBytes:  256 << 10,
		MaxInputBytes: 128 << 10,
		MaxInputDepth: 12,
		MaxInputNodes: 300,
		Timeout:       2 * time.Second,
	}
}

func NoCommandLimits() CommandLimits {
	return CommandLimits{
		MaxBodyBytes:  -1,
		MaxInputBytes: -1,
		MaxInputDepth: -1,
		MaxInputNodes: -1,
		Timeout:       -1,
	}
}

func (limits CommandLimits) withDefaults() CommandLimits {
	defaults := DefaultCommandLimits()
	if limits.MaxBodyBytes == 0 {
		limits.MaxBodyBytes = defaults.MaxBodyBytes
	}
	if limits.MaxInputBytes == 0 {
		limits.MaxInputBytes = defaults.MaxInputBytes
	}
	if limits.MaxInputDepth == 0 {
		limits.MaxInputDepth = defaults.MaxInputDepth
	}
	if limits.MaxInputNodes == 0 {
		limits.MaxInputNodes = defaults.MaxInputNodes
	}
	if limits.Timeout == 0 {
		limits.Timeout = defaults.Timeout
	}
	return limits
}

func (limits CommandLimits) disabled() bool {
	return limits.MaxBodyBytes < 0 &&
		limits.MaxInputBytes < 0 &&
		limits.MaxInputDepth < 0 &&
		limits.MaxInputNodes < 0
}

func validateCommandBodyLimits(body []byte, limits CommandLimits) error {
	if limits.disabled() {
		return nil
	}
	limits = limits.withDefaults()
	if limitExceeded(len(body), limits.MaxBodyBytes) {
		return fmt.Errorf("%w: command body size %d exceeds limit %d", ErrLimitExceeded, len(body), limits.MaxBodyBytes)
	}
	return nil
}

func validateCommandEnvelopeLimits(envelope CommandEnvelope, limits CommandLimits) error {
	if limits.disabled() {
		return nil
	}
	limits = limits.withDefaults()
	if limitExceeded(len(envelope.Input), limits.MaxInputBytes) {
		return fmt.Errorf("%w: command input size %d exceeds limit %d", ErrLimitExceeded, len(envelope.Input), limits.MaxInputBytes)
	}
	if len(envelope.Input) == 0 || string(envelope.Input) == "null" {
		return nil
	}

	var value any
	decoder := json.NewDecoder(bytes.NewReader(envelope.Input))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	state := commandParseState{limits: limits}
	return state.visitValue(value, 0)
}

type commandParseState struct {
	limits CommandLimits
	nodes  int
}

func (state *commandParseState) visitValue(value any, depth int) error {
	state.nodes++
	if state.limits.MaxInputNodes >= 0 && state.limits.MaxInputNodes > 0 && state.nodes > state.limits.MaxInputNodes {
		return fmt.Errorf("%w: command input node count %d exceeds limit %d", ErrLimitExceeded, state.nodes, state.limits.MaxInputNodes)
	}
	if state.limits.MaxInputDepth >= 0 && state.limits.MaxInputDepth > 0 && depth > state.limits.MaxInputDepth {
		return fmt.Errorf("%w: command input depth %d exceeds limit %d", ErrLimitExceeded, depth, state.limits.MaxInputDepth)
	}

	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if err := state.visitValue(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := state.visitValue(child, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
