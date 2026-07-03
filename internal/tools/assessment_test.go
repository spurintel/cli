package tools

import (
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

func TestValidateAssessmentRejectsMissingRiskParaphrase(t *testing.T) {
	t.Parallel()

	err := validateAssessment(FormatAssessmentInput{
		Context:    spur.IPContext{},
		BottomLine: "No geo mismatch observed.",
		Assessment: "The IP has otherwise limited signal.",
	})
	if err == nil || !strings.Contains(err.Error(), "geo mismatch") {
		t.Fatalf("expected geo mismatch validation error, got %v", err)
	}
}

func TestValidateAssessmentRejectsDensityPercentage(t *testing.T) {
	t.Parallel()

	err := validateAssessment(FormatAssessmentInput{
		Context: spur.IPContext{
			Client: spur.ClientInfo{
				Concentration: &spur.Concentration{Density: 0.42},
			},
		},
		BottomLine: "Treat with caution.",
		Assessment: "Client density is 42% near the observed city.",
	})
	if err == nil || !strings.Contains(err.Error(), "density is a 0-1 decimal") {
		t.Fatalf("expected density validation error, got %v", err)
	}
}

func TestRenderAssessmentEscapesDashboardQuery(t *testing.T) {
	t.Parallel()

	md := renderAssessment(FormatAssessmentInput{
		Context: spur.IPContext{
			IP: "2001:db8::1/odd?value",
		},
		BottomLine: "Treat with caution.",
		Assessment: "Observed tunnel signal.",
	})

	if !strings.Contains(md, "https://app.spur.us/search?q=2001%3Adb8%3A%3A1%2Fodd%3Fvalue") {
		t.Fatalf("dashboard link was not escaped: %s", md)
	}
}
