package main

import (
	"testing"
)

func TestSplitAndTrim_Empty(t *testing.T) {
	result := splitAndTrim("")
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestSplitAndTrim_Single(t *testing.T) {
	result := splitAndTrim("Linux/UNIX")
	if len(result) != 1 || result[0] != "Linux/UNIX" {
		t.Errorf("expected [Linux/UNIX], got %v", result)
	}
}

func TestSplitAndTrim_Multiple(t *testing.T) {
	result := splitAndTrim("us-east-1,us-west-2,eu-west-1")
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	expected := []string{"us-east-1", "us-west-2", "eu-west-1"}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("element %d: expected %q, got %q", i, v, result[i])
		}
	}
}

func TestSplitAndTrim_Whitespace(t *testing.T) {
	result := splitAndTrim(" spot , ondemand ")
	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}
	if result[0] != "spot" || result[1] != "ondemand" {
		t.Errorf("expected [spot ondemand], got %v", result)
	}
}

func TestCompileRegexes_Valid(t *testing.T) {
	regexes, err := compileRegexes([]string{"m5\\..*", "c5\\..*"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regexes) != 2 {
		t.Errorf("expected 2 regexes, got %d", len(regexes))
	}
}

func TestCompileRegexes_Invalid(t *testing.T) {
	_, err := compileRegexes([]string{"[invalid"})
	if err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestCompileRegexes_Empty(t *testing.T) {
	regexes, err := compileRegexes([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(regexes) != 0 {
		t.Errorf("expected 0 regexes, got %d", len(regexes))
	}
}

func TestValidateProductDesc(t *testing.T) {
	valid := []struct{ name, desc string }{
		{"Linux/UNIX", "Linux/UNIX"},
		{"Linux/UNIX VPC", "Linux/UNIX (Amazon VPC)"},
		{"SUSE Linux", "SUSE Linux"},
		{"SUSE Linux VPC", "SUSE Linux (Amazon VPC)"},
		{"Windows", "Windows"},
		{"Windows VPC", "Windows (Amazon VPC)"},
	}
	for _, tt := range valid {
		t.Run("valid/"+tt.name, func(t *testing.T) {
			if err := validateProductDesc([]string{tt.desc}); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}

	invalid := []string{"InvalidOS", "ubuntu", "Red Hat", ""}
	for _, desc := range invalid {
		t.Run("invalid/"+desc, func(t *testing.T) {
			if err := validateProductDesc([]string{desc}); err == nil {
				t.Errorf("expected error for %q, got nil", desc)
			}
		})
	}
}

func TestValidateOperatingSystems(t *testing.T) {
	valid := []string{"Linux", "RHEL", "SUSE", "Windows"}
	for _, os := range valid {
		t.Run("valid/"+os, func(t *testing.T) {
			if err := validateOperatingSystems([]string{os}); err != nil {
				t.Errorf("unexpected error for %q: %v", os, err)
			}
		})
	}

	invalid := []string{"macOS", "Ubuntu", "CentOS", ""}
	for _, os := range invalid {
		t.Run("invalid/"+os, func(t *testing.T) {
			if err := validateOperatingSystems([]string{os}); err == nil {
				t.Errorf("expected error for %q, got nil", os)
			}
		})
	}
}

func TestValidateSavingPlanTypes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "Compute", input: "Compute"},
		{name: "EC2Instance", input: "EC2Instance"},
		{name: "SageMaker", input: "SageMaker"},
		{name: "empty string (means none)", input: ""}, // allowed
		{name: "invalid", input: "InvalidPlan", wantErr: true},
		{name: "spot (not a plan type)", input: "spot", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSavingPlanTypes([]string{tt.input})
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for %q: %v", tt.input, err)
			}
		})
	}
}
