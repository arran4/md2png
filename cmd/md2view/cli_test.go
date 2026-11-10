package main

import (
	"strings"
	"testing"
)

func TestGeneratedParser(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectPtNil  bool
		expectPtVal  float64
		expectMargin int
	}{
		{
			name:         "omitted pt, explicit margin 0",
			args:         []string{"--margin", "0"},
			expectPtNil:  true,
			expectMargin: 0,
		},
		{
			name:        "explicit pt 0",
			args:        []string{"--pt", "0"},
			expectPtNil: false,
			expectPtVal: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, err := NewRoot("md2view", "test", "test", "test")
			if err != nil {
				t.Fatalf("NewRoot failed: %v", err)
			}

			executed := false
			cmd.CommandAction = func(c *RootCmd) error {
				executed = true
				if tc.expectPtNil {
					if c.pt != nil {
						t.Errorf("expected pt to be nil (omitted), got %v", *c.pt)
					}
				} else {
					if c.pt == nil {
						t.Errorf("expected pt to not be nil, but got nil")
					} else if *c.pt != tc.expectPtVal {
						t.Errorf("expected pt %v, got %v", tc.expectPtVal, *c.pt)
					}
				}

				if c.margin != nil {
					if *c.margin != tc.expectMargin {
						t.Errorf("expected margin %v, got %v", tc.expectMargin, *c.margin)
					}
				} else if tc.args[0] == "--margin" {
					t.Errorf("expected margin to not be nil")
				}
				return nil
			}

			if err := cmd.Execute(tc.args); err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if !executed {
				t.Errorf("CommandAction was never executed")
			}
		})
	}
}

func TestNoSeparatedFlagAlias(t *testing.T) {
	cmd, err := NewRoot("md2view", "test", "test", "test")
	if err != nil {
		t.Fatalf("NewRoot failed: %v", err)
	}
	err = cmd.Execute([]string{"--separated", "some/path.ttf"})
	if err == nil {
		t.Fatalf("expected error when passing nonexistent --separated flag, but got nil")
	}

	var buf strings.Builder
	_ = executeUsage(&buf, "md2view_usage.txt", UsageDataRootCmd{cmd, false})
	usage := buf.String()
	if strings.Contains(usage, "--separated") {
		t.Errorf("expected usage text to not advertise --separated, got:\n%s", usage)
	}
}
