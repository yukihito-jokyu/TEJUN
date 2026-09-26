package agentconnection

import (
	"testing"
	"time"
)

func TestProbeCanComplete(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		probe       Probe
		fingerprint string
		wantErr     bool
	}{
		{
			name: "authenticated",
			probe: Probe{
				ConfigFingerprint: "same", Compatibility: CompatibilityOK,
				AuthState: AuthAuthenticated, ExpiresAt: now.Add(time.Minute),
			},
			fingerprint: "same",
		},
		{
			name: "authentication not required",
			probe: Probe{
				ConfigFingerprint: "same", Compatibility: CompatibilityOK,
				AuthState: AuthNotRequired, ExpiresAt: now.Add(time.Minute),
			},
			fingerprint: "same",
		},
		{
			name: "expired",
			probe: Probe{
				ConfigFingerprint: "same", Compatibility: CompatibilityOK,
				AuthState: AuthAuthenticated, ExpiresAt: now,
			},
			fingerprint: "same", wantErr: true,
		},
		{
			name: "different fingerprint",
			probe: Probe{
				ConfigFingerprint: "other", Compatibility: CompatibilityOK,
				AuthState: AuthAuthenticated, ExpiresAt: now.Add(time.Minute),
			},
			fingerprint: "same", wantErr: true,
		},
		{
			name: "authentication required",
			probe: Probe{
				ConfigFingerprint: "same", Compatibility: CompatibilityOK,
				AuthState: AuthRequired, ExpiresAt: now.Add(time.Minute),
			},
			fingerprint: "same", wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.probe.CanComplete(now, tt.fingerprint); (got != nil) != tt.wantErr {
				t.Fatalf("CanComplete() error = %v, wantErr %v", got, tt.wantErr)
			}
		})
	}
}

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "job pending to running", got: CanTransitionJob(JobPending, JobRunning), want: true},
		{name: "job pending to succeeded", got: CanTransitionJob(JobPending, JobSucceeded), want: false},
		{
			name: "elicitation pending to responding",
			got:  CanTransitionElicitation(ElicitationPending, ElicitationResponding),
			want: true,
		},
		{
			name: "elicitation pending to responded",
			got:  CanTransitionElicitation(ElicitationPending, ElicitationResponded),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %v, want %v", tt.got, tt.want)
			}
		})
	}
}
