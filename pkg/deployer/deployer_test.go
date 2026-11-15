package deployer

import (
	"testing"
)

func TestComputeNewTargetValue(t *testing.T) {
	tests := []struct {
		oldVal        string
		path          string
		targetVersion string
		expected      string
	}{
		{
			oldVal:        "lscr.io/linuxserver/qbittorrent:latest",
			path:          "$.spec.template.spec.containers[0].image",
			targetVersion: "5.3.0_v2.1.2-ls485",
			expected:      "lscr.io/linuxserver/qbittorrent:5.3.0_v2.1.2-ls485",
		},
		{
			oldVal:        "filebrowser/filebrowser:latest",
			path:          "$.spec.template.spec.containers[0].image",
			targetVersion: "v2.63.9",
			expected:      "filebrowser/filebrowser:v2.63.9",
		},
		{
			oldVal:        "octoeverywhere/octoeverywhere:latest",
			path:          "$.spec.template.spec.containers[0].image",
			targetVersion: "5.4.1",
			expected:      "octoeverywhere/octoeverywhere:5.4.1",
		},
		{
			oldVal:        "alexxit/go2rtc",
			path:          "$.spec.template.spec.containers[0].image",
			targetVersion: "v1.9.4",
			expected:      "alexxit/go2rtc:v1.9.4",
		},
		{
			oldVal:        "0.15.3",
			path:          "$.spec.chart.spec.version",
			targetVersion: "v0.16.0",
			expected:      "v0.16.0",
		},
		{
			oldVal:        "v1.14.5",
			path:          "$.spec.chart.spec.version",
			targetVersion: "v1.21.3",
			expected:      "v1.21.3",
		},
		{
			oldVal:        "custom.registry.local:5000/my-app:1.0.0",
			path:          "$.spec.template.spec.containers[0].image",
			targetVersion: "1.1.0",
			expected:      "custom.registry.local:5000/my-app:1.1.0",
		},
	}

	for _, tc := range tests {
		got := computeNewTargetValue(tc.oldVal, tc.path, tc.targetVersion)
		if got != tc.expected {
			t.Errorf("computeNewTargetValue(%q, %q, %q) = %q; expected %q", tc.oldVal, tc.path, tc.targetVersion, got, tc.expected)
		}
	}
}
