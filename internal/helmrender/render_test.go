package helmrender

import (
	"errors"
	"testing"
)

func TestClassifyLocateChartError_MissingVersion(t *testing.T) {
	err := classifyLocateChartError(
		"oci://internal.oci.repo/helm-int/argo-cd",
		"",
		"1.2.3",
		errors.New("manifest unknown: requested version not found"),
	)

	versionErr, ok := err.(*MissingVersionError)
	if !ok {
		t.Fatalf("expected MissingVersionError, got %T", err)
	}
	if versionErr.TargetRevision != "1.2.3" {
		t.Fatalf("unexpected target revision %q", versionErr.TargetRevision)
	}
	if versionErr.ChartRef != "oci://internal.oci.repo/helm-int/argo-cd" {
		t.Fatalf("unexpected chart ref %q", versionErr.ChartRef)
	}
	if !IsMissingVersionError(err) {
		t.Fatalf("expected IsMissingVersionError to detect wrapped error")
	}
}

func TestClassifyLocateChartError_GenericError(t *testing.T) {
	original := errors.New("failed to fetch chart metadata")
	err := classifyLocateChartError("argo-cd", "https://charts.example.com", "1.2.3", original)
	if err != original {
		t.Fatalf("expected generic error to pass through unchanged")
	}
	if IsMissingVersionError(err) {
		t.Fatalf("did not expect missing version classification")
	}
}
