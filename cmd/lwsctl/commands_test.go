package main

import "testing"

func TestParseStopOptions(t *testing.T) {
	for _, test := range []struct {
		name      string
		options   []string
		recursive bool
		wantError bool
	}{
		{name: "default", options: nil},
		{name: "short", options: []string{"-r"}, recursive: true},
		{name: "long", options: []string{"--recursive"}, recursive: true},
		{name: "unknown", options: []string{"--purge"}, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			recursive, err := parseStopOptions(test.options)
			if (err != nil) != test.wantError {
				t.Fatalf("parseStopOptions() error = %v, wantError = %t", err, test.wantError)
			}
			if err == nil && recursive != test.recursive {
				t.Fatalf("parseStopOptions() recursive = %t, want %t", recursive, test.recursive)
			}
		})
	}
}

func TestParseDownOptions(t *testing.T) {
	recursive, purge, force, err := parseDownOptions([]string{"--recursive", "--purge", "-f"})
	if err != nil {
		t.Fatal(err)
	}
	if !recursive || !purge || !force {
		t.Fatalf("parseDownOptions() = recursive:%t purge:%t force:%t", recursive, purge, force)
	}

	if _, _, _, err := parseDownOptions([]string{"-r", "--unknown"}); err == nil {
		t.Fatal("不明なdownオプションを受理しました")
	}
}

func TestDomainPatternAllowsLocalhost(t *testing.T) {
	if !domainPattern.MatchString("localhost") {
		t.Fatal("localhostをベースドメインとして受理できません")
	}
}
