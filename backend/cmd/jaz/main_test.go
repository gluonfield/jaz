package main

import "testing"

func TestServerArgs(t *testing.T) {
	tests := []struct {
		name   string
		in     []string
		args   []string
		action mainAction
	}{
		{name: "bare runs server", action: mainRun},
		{name: "server flags", in: []string{"--addr", ":8080"}, args: []string{"--addr", ":8080"}, action: mainRun},
		{name: "serve alias", in: []string{"serve", "--addr", ":8080"}, args: []string{"--addr", ":8080"}, action: mainRun},
		{name: "server alias", in: []string{"server"}, action: mainRun},
		{name: "help", in: []string{"--help"}, action: mainHelp},
		{name: "serve help", in: []string{"serve", "--help"}, action: mainHelp},
		{name: "chat is not a subcommand", in: []string{"chat"}, action: mainInvalid},
		{name: "unknown subcommand", in: []string{"worker"}, action: mainInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, action := serverArgs(tt.in)
			if action != tt.action {
				t.Fatalf("action = %v, want %v", action, tt.action)
			}
			if !sameArgs(args, tt.args) {
				t.Fatalf("args = %#v, want %#v", args, tt.args)
			}
		})
	}
}

func sameArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
