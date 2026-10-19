package extserver

import (
	extv1 "github.com/heartblast/ableops-sdk/extension/v1"
	"testing"
)

func TestHostSelectedDataDirIsOptionalAndValidated(t *testing.T) {
	manifest := extv1.Manifest{ID: "sample", Version: "1.0.0"}
	values := map[string]string{EnvExtensionID: "sample", EnvCallToken: "call", EnvHostURL: "http://127.0.0.1:1234", EnvHostToken: "host"}
	lookup := func(key string) (string, bool) { v, ok := values[key]; return v, ok }
	env, err := loadEnvironment(lookup, manifest)
	if err != nil || env.DataDir != "" {
		t.Fatalf("old host: %v", err)
	}
	values[EnvDataDir] = t.TempDir()
	env, err = loadEnvironment(lookup, manifest)
	if err != nil || env.DataDir != values[EnvDataDir] {
		t.Fatalf("data dir unavailable: %v", err)
	}
	host, _ := newHostContext(env, nil, 0)
	if host.DataDir != values[EnvDataDir] {
		t.Fatal("HostContext lost dataDir")
	}
	values[EnvDataDir] = "../package-data"
	if _, err := loadEnvironment(lookup, manifest); err == nil {
		t.Fatal("relative arbitrary data path accepted")
	}
}
