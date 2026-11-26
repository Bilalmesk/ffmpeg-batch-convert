package quality

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVMAF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vmaf.json")
	body := `{
	  "frames": [
	    {"metrics": {"vmaf": 90.0}},
	    {"metrics": {"vmaf": 100.0}}
	  ],
	  "pooled_metrics": {
	    "vmaf": {"min": 90.0, "max": 100.0, "mean": 95.0, "harmonic_mean": 94.7}
	  }
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var result Result
	if err := parseVMAF(path, &result); err != nil {
		t.Fatal(err)
	}
	if result.Frames != 2 || result.Score != 95 || result.Min != 90 || result.Max != 100 {
		t.Fatalf("result = %+v", result)
	}
	if result.HarmonicMean != 94.7 {
		t.Fatalf("harmonic mean = %v", result.HarmonicMean)
	}
}

func TestParseVMAFFallbackPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vmaf.json")
	body := `{"frames":[{"metrics":{"vmaf":80}},{"metrics":{"vmaf":100}}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var result Result
	if err := parseVMAF(path, &result); err != nil {
		t.Fatal(err)
	}
	if result.Score != 90 || result.Min != 80 || result.Max != 100 {
		t.Fatalf("result = %+v", result)
	}
}

func TestParseVMAFEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vmaf.json")
	if err := os.WriteFile(path, []byte(`{"frames":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var result Result
	if err := parseVMAF(path, &result); err == nil {
		t.Fatal("expected an error for an empty VMAF log")
	}
}

func TestBuildFilter(t *testing.T) {
	filter, level, log := buildFilter(Request{Metric: MetricVMAF, RefWidth: 1920, RefHeight: 1080, Threads: 4})
	if !strings.Contains(filter, "scale=1920:1080") || !strings.Contains(filter, "libvmaf") {
		t.Fatalf("filter = %q", filter)
	}
	if !strings.Contains(filter, "n_threads=4") || log != "vmaf.json" || level != "error" {
		t.Fatalf("filter = %q level=%q log=%q", filter, level, log)
	}
	ssim, level, _ := buildFilter(Request{Metric: MetricSSIM})
	if !strings.HasSuffix(ssim, "ssim") || level != "info" {
		t.Fatalf("ssim filter = %q level=%q", ssim, level)
	}
}

func TestMatchNumbers(t *testing.T) {
	if value, ok := matchNumber("[Parsed_ssim_0] SSIM All:0.991234 (20.4)", `All:([0-9.]+)`); !ok || value != 0.991234 {
		t.Fatalf("ssim value = %v ok=%v", value, ok)
	}
	if got := matchText("PSNR average:42.123 min:1 max:99", `average:([^\s]+)`); got != "42.123" {
		t.Fatalf("psnr = %q", got)
	}
	if got := matchText("PSNR average:inf", `average:([^\s]+)`); got != "inf" {
		t.Fatalf("psnr inf = %q", got)
	}
}

func TestParseMetric(t *testing.T) {
	for input, want := range map[string]Metric{"": MetricVMAF, "VMAF": MetricVMAF, "ssim": MetricSSIM, "PSNR": MetricPSNR} {
		got, ok := ParseMetric(input)
		if !ok || got != want {
			t.Fatalf("ParseMetric(%q) = %q, %v", input, got, ok)
		}
	}
	if _, ok := ParseMetric("butteraugli"); ok {
		t.Fatal("unknown metric should fail")
	}
}
