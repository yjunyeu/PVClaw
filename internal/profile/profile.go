// Package profile loads and validates PVClaw Profile V1 documents.
package profile

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const Version1 = 1

var agentIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

type Profile struct {
	Version      int          `yaml:"version"`
	Agent        Agent        `yaml:"agent"`
	Data         Data         `yaml:"data"`
	Network      Network      `yaml:"network"`
	Capabilities Capabilities `yaml:"capabilities"`
	Policy       Policy       `yaml:"policy"`
}

type Agent struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Purpose string `yaml:"purpose"`
}

type Data struct {
	Access string `yaml:"access"`
}

type Network struct {
	Enabled bool `yaml:"enabled"`
}

type Capabilities struct {
	Shell              bool `yaml:"shell"`
	WebSearch          bool `yaml:"web_search"`
	Browser            bool `yaml:"browser"`
	SimulatedMessaging bool `yaml:"simulated_messaging"`
}

type Policy struct {
	Profile string `yaml:"profile"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	parts := make([]string, 0, len(e))
	for _, issue := range e {
		parts = append(parts, issue.Error())
	}
	return strings.Join(parts, "; ")
}

func LoadFile(path string) (Profile, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("read profile: %w", err)
	}
	return Parse(contents)
}

func Parse(contents []byte) (Profile, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	var p Profile
	if err := decoder.Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("parse profile YAML: %w", err)
	}
	return p, nil
}

func Validate(p Profile) error {
	issues := ValidationErrors{}
	if p.Version != Version1 {
		issues = append(issues, ValidationError{Field: "version", Message: "must be 1"})
	}
	if strings.TrimSpace(p.Agent.ID) == "" {
		issues = append(issues, ValidationError{Field: "agent.id", Message: "is required"})
	} else if !ValidAgentID(p.Agent.ID) {
		issues = append(issues, ValidationError{
			Field: "agent.id", Message: "must use lowercase letters, numbers, dashes, or underscores",
		})
	}
	if strings.TrimSpace(p.Agent.Name) == "" {
		issues = append(issues, ValidationError{Field: "agent.name", Message: "is required"})
	}
	if strings.TrimSpace(p.Agent.Purpose) == "" {
		issues = append(issues, ValidationError{Field: "agent.purpose", Message: "is required"})
	}
	if p.Data.Access != "read-only" && p.Data.Access != "read-write" {
		issues = append(issues, ValidationError{
			Field: "data.access", Message: "must be read-only or read-write",
		})
	}
	if strings.TrimSpace(p.Policy.Profile) == "" {
		issues = append(issues, ValidationError{Field: "policy.profile", Message: "is required"})
	}
	if len(issues) > 0 {
		return issues
	}
	return nil
}

// ValidAgentID reports whether an ID is safe for PVClaw and downstream OpenClaw use.
func ValidAgentID(id string) bool {
	return agentIDPattern.MatchString(id)
}
