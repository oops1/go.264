//go:build linux && (amd64 || arm64)

package vaapi

import "testing"

func TestTheBackendOffersEncodingUnderItsName(t *testing.T) {
	b := Backend()
	if b.Name != backendName || b.ProbeEncode == nil || b.ProbeDecode != nil {
		t.Fatalf("Backend() = %q with encode probe %v and decode probe %v, want %q encoding only",
			b.Name, b.ProbeEncode != nil, b.ProbeDecode != nil, backendName)
	}
}
