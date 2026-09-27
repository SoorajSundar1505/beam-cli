package main

import "testing"

func TestShouldDetachConsole(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		procs int
		want  bool
	}{
		{name: "status in cmd", args: []string{"beam.exe", "status"}, procs: 2, want: false},
		{name: "devices in powershell", args: []string{"beam.exe", "devices"}, procs: 2, want: false},
		{name: "root in cmd", args: []string{"beam.exe"}, procs: 2, want: false},
		{name: "foreground daemon", args: []string{"beam.exe", "daemon"}, procs: 2, want: false},
		{name: "login daemon", args: []string{"beam.exe", "daemon"}, procs: 1, want: true},
		{name: "hidden daemon child", args: []string{"beam.exe", "daemon"}, procs: 0, want: false},
		{name: "daemon flag without command", args: []string{"beam.exe", "--background"}, procs: 1, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldDetachConsole(tc.args, tc.procs); got != tc.want {
				t.Fatalf("shouldDetachConsole(%v, %d) = %v", tc.args, tc.procs, got)
			}
		})
	}
}
