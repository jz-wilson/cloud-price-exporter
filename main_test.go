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

func TestValidateProductDesc_AllValid(t *testing.T) {
	valid := []string{"Linux/UNIX", "SUSE Linux", "Windows", "Linux/UNIX (Amazon VPC)", "SUSE Linux (Amazon VPC)", "Windows (Amazon VPC)"}
	if err := validateProductDesc(valid); err != nil {
		t.Errorf("unexpected error for valid descriptions: %v", err)
	}
}

func TestValidateProductDesc_Invalid(t *testing.T) {
	if err := validateProductDesc([]string{"InvalidOS"}); err == nil {
		t.Error("expected error for invalid product description")
	}
}

func TestValidateOperatingSystems_AllValid(t *testing.T) {
	valid := []string{"Linux", "RHEL", "SUSE", "Windows"}
	if err := validateOperatingSystems(valid); err != nil {
		t.Errorf("unexpected error for valid OS: %v", err)
	}
}

func TestValidateOperatingSystems_Invalid(t *testing.T) {
	if err := validateOperatingSystems([]string{"macOS"}); err == nil {
		t.Error("expected error for invalid operating system")
	}
}

func TestValidateSavingPlanTypes_AllValid(t *testing.T) {
	valid := []string{"Compute", "EC2Instance", "SageMaker"}
	if err := validateSavingPlanTypes(valid); err != nil {
		t.Errorf("unexpected error for valid plan types: %v", err)
	}
}

func TestValidateSavingPlanTypes_Empty(t *testing.T) {
	// Empty string is allowed (means "none")
	if err := validateSavingPlanTypes([]string{""}); err != nil {
		t.Errorf("unexpected error for empty plan type: %v", err)
	}
}

func TestValidateSavingPlanTypes_Invalid(t *testing.T) {
	if err := validateSavingPlanTypes([]string{"InvalidPlan"}); err == nil {
		t.Error("expected error for invalid saving plan type")
	}
}
