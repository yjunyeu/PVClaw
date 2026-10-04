package profile_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pvclaw/pvclaw/internal/profile"
)

func TestExampleProfilesAreValid(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"medical.example.yaml", "work.example.yaml"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			contents, err := os.ReadFile(filepath.Join("../../profiles", name))
			if err != nil {
				t.Fatal(err)
			}
			p, err := profile.Parse(contents)
			if err != nil {
				t.Fatal(err)
			}
			if err := profile.Validate(p); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidationRejectsInvalidProfiles(t *testing.T) {
	t.Parallel()
	valid := profile.Profile{
		Version: 1,
		Agent:   profile.Agent{ID: "medical", Name: "Medical", Purpose: "medical"},
		Data:    profile.Data{Access: "read-only"},
		Policy:  profile.Policy{Profile: "medical"},
	}
	tests := []struct {
		name   string
		mutate func(*profile.Profile)
		field  string
	}{
		{"invalid version", func(p *profile.Profile) { p.Version = 2 }, "version"},
		{"invalid agent ID", func(p *profile.Profile) { p.Agent.ID = "Medical Agent" }, "agent.id"},
		{"invalid access mode", func(p *profile.Profile) { p.Data.Access = "write" }, "data.access"},
		{"missing policy profile", func(p *profile.Profile) { p.Policy.Profile = " " }, "policy.profile"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.mutate(&p)
			err := profile.Validate(p)
			if err == nil {
				t.Fatal("Validate() succeeded unexpectedly")
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("error %q does not identify %q", err, tt.field)
			}
		})
	}
}
