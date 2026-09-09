package main

import (
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
